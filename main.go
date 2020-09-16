/*
 * Copyright 2020 Yubico AB
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

type commentData struct {
	Fails   []string
	Warns   []string
	DocsURL string
}

const commentTemplate = `**Conftest has identified issues with your resources**
{{ if .Fails }}
The following policy violations were identified. These are blocking and must be remediated before proceeding.

{{ range .Fails }}* {{ . }}
{{ end }}{{ end }}{{ if .Warns }}
The following warnings were identified. These are issues that indicate the resources are not following best practices.

{{ range .Warns }}* {{ . }}
{{ end }}{{ end }}
{{ if .DocsURL }}For more information, see the [policy documentation]({{ .DocsURL }}).
{{end}}`

var (
	conftestFlags = []string{"COMBINE", "POLICY", "ALL_NAMESPACES", "DATA"}
)

func main() {
	if os.Getenv("FILES") == "" {
		errorAndExit(fmt.Errorf("at least one file to test must be supplied"))
	}

	pullURL, err := getFullPullURL()
	if err != nil {
		errorAndExit(err)
	}

	if pullURL != "" {
		if err := runConftestPull(pullURL); err != nil {
			errorAndExit(err)
		}
	}

	fails, warns, err := runConftestTest()
	if err != nil {
		errorAndExit(fmt.Errorf("running conftest: %w", err))
	}

	if len(fails) > 0 {
		defer os.Exit(1)
	}

	if os.Getenv("ADD_COMMENT") != "true" {
		return
	}
	if len(fails) == 0 && len(warns) == 0 {
		return
	}

	d := commentData{Fails: fails, Warns: warns}
	if os.Getenv("DOCS_URL") != "" {
		d.DocsURL = os.Getenv("DOCS_URL")
	}

	c, err := getCommentJSON(d)
	if err != nil {
		errorAndExit(fmt.Errorf("getting comment: %w", err))
	}

	if err := submitComment(c); err != nil {
		errorAndExit(fmt.Errorf("submitting comment: %w", err))
	}
}

func getFullPullURL() (string, error) {
	pullURL := os.Getenv("PULL_URL")
	if pullURL == "" {
		return "", nil
	}

	pullURLSplit := strings.Split(pullURL, "/")
	if len(pullURLSplit) == 1 {
		return "", fmt.Errorf("invalid url: %s", pullURL)
	}

	pullSecret := os.Getenv("PULL_SECRET")
	if pullSecret == "" {
		return pullURL, nil
	}

	pullURI := pullURLSplit[0]
	switch pullURI {
	case "gcs::https:":
		if err := ioutil.WriteFile("gcs.json", []byte(pullSecret), os.ModePerm); err != nil {
			return "", fmt.Errorf("writing gcs creds: %w", err)
		}
		os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "gcs.json")

	case "s3::https:":
		pullURL = pullURL + "?" + pullSecret

	case "https:":
		u, err := url.Parse(pullURL)
		if err != nil {
			return "", fmt.Errorf("parsing url: %w", err)
		}
		pullURL = "https://" + pullSecret + "@" + u.Host + u.Path

	default:
		return "", fmt.Errorf("PULL_SECRET not supported with uri: %s", pullURI)
	}

	return pullURL, nil
}

func runConftestPull(url string) error {
	cmd := exec.Command("conftest", "pull", url)
	var out bytes.Buffer
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running conftest pull: %s", out.String())
	}

	return nil
}

func runConftestTest() ([]string, []string, error) {
	args := []string{"test", "--no-color"}
	flags := getFlagsFromEnv()
	args = append(args, flags...)
	files := strings.Split(os.Getenv("FILES"), " ")
	args = append(args, files...)

	var out bytes.Buffer
	cmd := exec.Command("conftest", args...)
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Run()

	lines := strings.Split(out.String(), "\n")
	if strings.HasPrefix(lines[0], "Error:") {
		return nil, nil, fmt.Errorf("invalid flags: %s", lines[0])
	}

	var fails, warns []string
	for _, l := range lines {
		if strings.HasPrefix(l, "WARN") {
			warns = append(warns, l)
			continue
		}

		if strings.HasPrefix(l, "FAIL") {
			fails = append(fails, l)
		}
	}

	fmt.Println(out.String())

	return fails, warns, nil
}

func getFlagsFromEnv() []string {
	var args []string
	for _, v := range conftestFlags {
		env := os.Getenv(v)
		if env == "" || strings.ToLower(env) == "false" {
			continue
		}

		flag := getFlagFromEnv(v)
		if strings.ToLower(env) == "true" {
			args = append(args, flag)
		} else {
			args = append(args, flag, env)
		}
	}

	return args
}

func getCommentJSON(d commentData) ([]byte, error) {
	t, err := template.New("conftest").Parse(commentTemplate)
	if err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	var o bytes.Buffer
	if err := t.Execute(&o, d); err != nil {
		return nil, fmt.Errorf("executing template: %w", err)
	}

	j, err := json.Marshal(map[string]string{"body": o.String()})
	if err != nil {
		return nil, fmt.Errorf("marshalling comment")
	}

	return j, nil
}

func submitComment(comment []byte) error {
	req, err := http.NewRequest("POST", os.Getenv("GITHUB_COMMENT_URL"), bytes.NewReader(comment))
	if err != nil {
		return fmt.Errorf("creating http request: %w", err)
	}
	req.Header.Add("Accept", "application/vnd.github.v3+json")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("token %s", os.Getenv("GITHUB_TOKEN")))

	c := http.Client{}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("submitting http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		msg, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("submitting comment: %s", string(msg))
	}

	return nil
}

func getFlagFromEnv(e string) string {
	return fmt.Sprintf("--%s", strings.ToLower(strings.ReplaceAll(e, "_", "-")))
}

func errorAndExit(e error) {
	fmt.Println(e)
	os.Exit(1)
}
