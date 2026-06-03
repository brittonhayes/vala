Record a piece of threat intelligence in the brain.

Intel is a first-class artifact: an `indicator` (IOC such as a hash, IP, domain,
or URL), a `ttp` (a technique, ideally a MITRE ATT&CK ID), an `actor` (a threat
group), or a `narrative` (a short writeup). Supply the `kind` and a `value`.

The tool returns an intel ID. During a hunt the intel is linked to that hunt
automatically; use `link_artifacts` to connect it to alerts or detections. Intel
recorded here can later be fed into detection development.
