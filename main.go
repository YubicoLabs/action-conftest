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

type jsonResult struct {
	Message  string                 `json:"msg"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type jsonCheckResult struct {
	Filename  string       `json:"filename"`
	Successes []jsonResult `json:"successes"`
	Warnings  []jsonResult `json:"warnings,omitempty"`
	Failures  []jsonResult `json:"failures,omitempty"`
}

type metricsSubmission struct {
	SourceID  string            `json:"sourceID"`
	Successes int               `json:"successes,omitempty"`
	Warnings  metricsSeverity   `json:"warns,omitempty"`
	Failures  metricsSeverity   `json:"fails,omitempty"`
	Details   []jsonCheckResult `json:"details,omitempty"`
}

type metricsSeverity struct {
	Count     int      `json:"count"`
	PolicyIDs []string `json:"policyIDs"`
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

var conftestFlags = []string{"COMBINE", "POLICY", "ALL_NAMESPACES", "DATA"}

func main() {
	err := run()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	if os.Getenv("FILES") == "" {
		return fmt.Errorf("at least one file to test must be supplied")
	}

	pullURL, err := getFullPullURL()
	if err != nil {
		return fmt.Errorf("get full pull url: %w", err)
	}

	if pullURL != "" {
		if err := runConftestPull(pullURL); err != nil {
			return fmt.Errorf("runnning conftest pull: %w", err)
		}
	}

	results, err := runConftestTest()
	if err != nil {
		return fmt.Errorf("running conftest: %w", err)
	}

	metricsURL := os.Getenv("METRICS_URL")
	policyIDKey := os.Getenv("POLICY_ID_KEY")

	var policiesWithFails, policiesWithWarns []string
	var fails, warns []string
	var successes int
	for _, result := range results {
		successes += len(result.Successes)

		for _, fail := range result.Failures {
			// attempt to parse the policy ID section, skip if there are errors
			policyID, err := getPolicyIDFromMetadata(fail.Metadata, policyIDKey)
			if err != nil {
				fails = append(fails, fmt.Sprintf("%s - %s", result.Filename, fail.Message))
				continue
			}

			fails = append(fails, fmt.Sprintf("%s - %s: %s", result.Filename, policyID, fail.Message))

			if !contains(policiesWithFails, policyID) {
				policiesWithFails = append(policiesWithFails, policyID)
			}
		}

		for _, warn := range result.Warnings {
			// attempt to parse the policy ID section, skip if there are errors
			policyID, err := getPolicyIDFromMetadata(warn.Metadata, policyIDKey)
			if err != nil {
				warns = append(warns, fmt.Sprintf("%s - %s", result.Filename, warn.Message))
				continue
			}

			warns = append(warns, fmt.Sprintf("%s - %s: %s", result.Filename, policyID, warn.Message))

			if !contains(policiesWithWarns, policyID) {
				policiesWithWarns = append(policiesWithWarns, policyID)
			}
		}
	}

	// attempt to submit metrics, but do not fail the CI job if there are errors
	if metricsURL != "" {
		sourceID := os.Getenv("METRICS_SOURCE")
		if sourceID == "" {
			return fmt.Errorf("metrics-source must be specified if metrics-url is set")
		}

		metrics := metricsSubmission{
			SourceID:  sourceID,
			Successes: successes,
			Failures: metricsSeverity{
				Count:     len(fails),
				PolicyIDs: policiesWithFails,
			},
			Warnings: metricsSeverity{
				Count:     len(warns),
				PolicyIDs: policiesWithWarns,
			},
		}
		if strings.ToLower(os.Getenv("METRICS_DETAILS")) == "true" {
			metrics.Details = results
		}
		metricsJSON, err := json.Marshal(metrics)
		if err != nil {
			return fmt.Errorf("marshal metrics json: %w", err)
		}

		var metricsToken string
		if os.Getenv("METRICS_TOKEN") != "" {
			metricsToken = fmt.Sprintf("Bearer %s", os.Getenv("METRICS_TOKEN"))
		}

		submitPost(metricsURL, metricsJSON, metricsToken)
	}

	if len(fails) == 0 && len(warns) == 0 {
		fmt.Println("No policy violations or warnings were identified.")
		return nil
	}

	d := commentData{Fails: fails, Warns: warns}
	if os.Getenv("DOCS_URL") != "" {
		d.DocsURL = os.Getenv("DOCS_URL")
	}

	t, err := renderTemplate(d)
	if err != nil {
		return fmt.Errorf("rendering template: %w", err)
	}

	// ensure the results are written to the CI logs
	fmt.Println(string(t))

	if os.Getenv("ADD_COMMENT") != "true" {
		return nil
	}

	ghComment, err := getCommentJSON(t)
	if err != nil {
		return fmt.Errorf("get comment json: %w", err)
	}

	ghToken := fmt.Sprintf("token %s", os.Getenv("GITHUB_TOKEN"))
	if err := submitPost(os.Getenv("GITHUB_COMMENT_URL"), ghComment, ghToken); err != nil {
		return fmt.Errorf("submitting comment: %w", err)
	}

	if len(fails) > 0 {
		if strings.ToLower(os.Getenv("NO_FAIL")) != "true" {
			return fmt.Errorf("%d policy violations were found", len(fails))
		}
	}

	return nil
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
		return fmt.Errorf("%s", out.String())
	}

	return nil
}

func runConftestTest() ([]jsonCheckResult, error) {
	args := []string{"test", "--no-color", "--output", "json"}
	flags := getFlagsFromEnv()
	args = append(args, flags...)
	files := strings.Split(os.Getenv("FILES"), " ")
	args = append(args, files...)

	cmd := exec.Command("conftest", args...)
	out, _ := cmd.CombinedOutput() // intentionally ignore errors so we can parse the results

	var results []jsonCheckResult
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("%s", string(out))
	}

	return results, nil
}

func getPolicyIDFromMetadata(metadata map[string]interface{}, policyIDKey string) (string, error) {
	details := metadata["details"].(map[string]interface{})
	if details[policyIDKey] == "" {
		return "", fmt.Errorf("empty policyID key")
	}

	return fmt.Sprintf("%v", details[policyIDKey]), nil
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

func renderTemplate(d commentData) ([]byte, error) {
	t, err := template.New("conftest").Parse(commentTemplate)
	if err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	var o bytes.Buffer
	if err := t.Execute(&o, d); err != nil {
		return nil, fmt.Errorf("executing template: %w", err)
	}

	return o.Bytes(), nil
}

func getCommentJSON(comment []byte) ([]byte, error) {
	j, err := json.Marshal(map[string]string{"body": string(comment)})
	if err != nil {
		return nil, fmt.Errorf("marshalling comment: %w", err)
	}

	return j, nil
}

func submitPost(url string, data []byte, authz string) error {
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating http request: %w", err)
	}

	req.Header.Add("Content-Type", "application/json")
	if authz != "" {
		req.Header.Add("Authorization", authz)
	}

	c := http.Client{}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("submitting http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			msg = []byte(fmt.Sprintf("unable to read response body: %s", err))
		}

		return fmt.Errorf("remote server error: status %d: %s", resp.StatusCode, string(msg))
	}

	return nil
}

func getFlagFromEnv(e string) string {
	return fmt.Sprintf("--%s", strings.ToLower(strings.ReplaceAll(e, "_", "-")))
}

func contains(list []string, item string) bool {
	for _, l := range list {
		if l == item {
			return true
		}
	}

	return false
}
