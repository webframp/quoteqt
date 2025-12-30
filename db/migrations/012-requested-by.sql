-- Add requested_by to track who originally requested a quote (e.g., chat user who suggested it)
ALTER TABLE quotes ADD COLUMN requested_by TEXT;

-- Add submitted_by_user to track who submitted the suggestion
ALTER TABLE quote_suggestions ADD COLUMN submitted_by_user TEXT;
