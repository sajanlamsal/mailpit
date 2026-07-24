-- INDEX the authenticated username stored in the message metadata.
-- Without this, listing/counting messages per mailbox (and building the
-- mailbox list) requires a full table scan evaluating json_extract per row.
CREATE INDEX IF NOT EXISTS {{ tenant "idx_mailbox_username" }}
	ON {{ tenant "mailbox" }} (json_extract(Metadata, '$.Username'));
