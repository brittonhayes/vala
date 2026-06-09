Connect two brain artifacts by setting a relation.

Pass `from_id` (the row to update), a `relation` (one of evidence, intel, hunts,
detections), and `to_ids` (the rows to link to). This is how the brain becomes a
connected graph: link intel to the hunt that surfaced it, or a detection to the
intel and hunt that inform it.
