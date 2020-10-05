package always_warn

warn[msg] {
    msg := {
        "msg": "a warning! you should probably fix this",
        "details": {"policyID": "P0000"}
    }

    true
}
