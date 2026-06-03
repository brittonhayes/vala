Connect two brain artifacts by setting a relation.

Pass `from_id` (the row to update), a `relation` (one of evidence, intel, hunts,
alerts, detections, case), and `to_ids` (the rows to link to). This is how the
brain becomes a connected graph: link intel to the hunt that surfaced it, a hunt
to the alert it explains, or a detection to the intel that informs it.
