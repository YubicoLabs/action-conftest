package always_warn

warn[msg] {
    msg := format_with_id("a warning! you should probably fix this", "P0000")
    true
}

format_with_id(msg, id) = msg_fmt {
    msg_fmt := {
        "msg": sprintf("%s: %s", [id, msg]),
        "details": {"policyID": id}
    }
}
