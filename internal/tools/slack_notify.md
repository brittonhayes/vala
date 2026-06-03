Send a Slack notification. This is a side-effecting action: it can only run in
the execute phase, after the corresponding proposal has been approved.

Provide the `message` to post. Do not include credentials or secrets in the
message.
