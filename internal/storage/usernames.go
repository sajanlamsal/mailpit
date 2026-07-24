package storage

import (
	"context"
	"database/sql"

	"github.com/axllent/mailpit/internal/logger"
	"github.com/leporo/sqlf"
)

// usernameSQL is the JSON path expression used to extract the authenticated
// username stored in a message's metadata.
const usernameSQL = `json_extract(Metadata, '$.Username')`

// GetAllUsernames returns all distinct SMTP/Send-API authentication usernames
// currently in use, sorted alphabetically. Messages received without
// authentication (blank username) are excluded.
func GetAllUsernames() []string {
	var usernames = []string{}
	var name string

	if err := sqlf.
		Select(`DISTINCT `+usernameSQL).To(&name).
		From(tenant("mailbox")).
		Where(usernameSQL+` IS NOT NULL`).
		Where(usernameSQL+` != ''`).
		OrderBy(usernameSQL).
		QueryAndClose(context.TODO(), db, func(_ *sql.Rows) {
			usernames = append(usernames, name)
		}); err != nil {
		logger.Log().Errorf("[db] %s", err.Error())
	}

	return usernames
}

// CountTotalForUsername returns the number of messages belonging to a mailbox
// (ie: received from the given authenticated username).
func CountTotalForUsername(username string) uint64 {
	var total float64 // use float64 for rqlite compatibility

	_ = sqlf.From(tenant("mailbox")).
		Select("COUNT(*)").To(&total).
		Where(usernameSQL+" = ?", username).
		QueryRowAndClose(context.TODO(), db)

	return uint64(total)
}

// CountUnreadForUsername returns the number of unread messages belonging to a mailbox.
func CountUnreadForUsername(username string) uint64 {
	var total float64 // use float64 for rqlite compatibility

	_ = sqlf.From(tenant("mailbox")).
		Select("COUNT(*)").To(&total).
		Where(usernameSQL+" = ?", username).
		Where("Read = ?", 0).
		QueryRowAndClose(context.TODO(), db)

	return uint64(total)
}

// GetAllTagsForUsername returns only the tags used by messages belonging to a
// mailbox, sorted alphabetically. This keeps the tag list scoped to the mailbox
// being viewed rather than exposing every tag in the database.
func GetAllTagsForUsername(username string) []string {
	var tags = []string{}
	var name string

	if err := sqlf.
		Select(`DISTINCT t.Name`).To(&name).
		From(tenant("tags")+` t`).
		Join(tenant("message_tags")+` mt`, `mt.TagID = t.ID`).
		Join(tenant("mailbox")+` m`, `m.ID = mt.ID`).
		Where(`json_extract(m.Metadata, '$.Username') = ?`, username).
		OrderBy(`t.Name`).
		QueryAndClose(context.TODO(), db, func(_ *sql.Rows) {
			tags = append(tags, name)
		}); err != nil {
		logger.Log().Errorf("[db] %s", err.Error())
	}

	return tags
}

// StatsGetForUsername returns the total/unread/tag statistics scoped to a single
// mailbox, rather than the whole database.
func StatsGetForUsername(username string) MailboxStats {
	return MailboxStats{
		Total:     CountTotalForUsername(username),
		Unread:    CountUnreadForUsername(username),
		Tags:      GetAllTagsForUsername(username),
		Usernames: GetAllUsernames(),
	}
}
