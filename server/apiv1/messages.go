package apiv1

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/axllent/mailpit/internal/storage"
	"github.com/axllent/mailpit/internal/tools"
)

// mailboxSearch builds the search expression that restricts results to a single
// mailbox (ie: messages received from that authenticated username). Usernames
// containing whitespace are quoted so they are parsed as a single search term.
func mailboxSearch(name string) string {
	if strings.ContainsAny(name, " \t") {
		return `username:"` + name + `"`
	}

	return "username:" + name
}

// MessagesSummary is a summary of a list of messages
type MessagesSummary struct {
	// Total number of messages in mailbox
	Total uint64 `json:"total"`

	// Total number of unread messages in mailbox
	Unread uint64 `json:"unread"`

	// Legacy - now undocumented in API specs but left for backwards compatibility.
	// Removed from API documentation 2023-07-12
	// swagger:ignore
	Count uint64 `json:"count"`

	// Total number of messages matching current query
	MessagesCount uint64 `json:"messages_count"`

	// Total number of unread messages matching current query
	MessagesUnreadCount uint64 `json:"messages_unread"`

	// Pagination offset
	Start int `json:"start"`

	// All current tags
	Tags []string `json:"tags"`

	// All current authentication usernames (SMTP / Send API) in use
	Usernames []string `json:"usernames"`

	// Messages summary
	// in: body
	Messages []storage.MessageSummary `json:"messages"`
}

// GetMessages returns a paginated list of messages as JSON
func GetMessages(w http.ResponseWriter, r *http.Request) {
	// swagger:route GET /api/v1/messages messages GetMessagesParams
	//
	// # List messages
	//
	// Returns messages from the mailbox ordered from newest to oldest.
	//
	//	Produces:
	//	  - application/json
	//
	//	Schemes: http, https
	//
	//	Responses:
	//	  200: MessagesSummaryResponse
	//    400: ErrorResponse

	start, beforeTS, limit := getStartLimit(r)

	// optionally restrict the listing to a single mailbox (authenticated username)
	mailboxName := strings.TrimSpace(r.URL.Query().Get("mailbox"))

	var (
		messages []storage.MessageSummary
		stats    storage.MailboxStats
		err      error
	)

	if mailboxName != "" {
		// scope both the messages and the statistics to the mailbox
		messages, _, err = storage.Search(mailboxSearch(mailboxName), "", start, beforeTS, limit)
		stats = storage.StatsGetForUsername(mailboxName)
	} else {
		messages, err = storage.List(start, beforeTS, limit)
		stats = storage.StatsGet()
	}

	if err != nil {
		httpError(w, err.Error())
		return
	}

	var res MessagesSummary

	res.Start = start
	res.Messages = messages
	res.Count = uint64(len(messages)) // legacy - now undocumented in API specs
	res.Total = stats.Total
	res.Unread = stats.Unread
	res.Tags = stats.Tags
	res.Usernames = stats.Usernames
	res.MessagesCount = stats.Total
	res.MessagesUnreadCount = stats.Unread

	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		httpError(w, err.Error())
	}
}

// SetReadStatus (method: PUT) will update the status to Read/Unread for all provided IDs.
func SetReadStatus(w http.ResponseWriter, r *http.Request) {
	// swagger:route PUT /api/v1/messages messages SetReadStatusParams
	//
	// # Set read status
	//
	// You can optionally provide an array of IDs or a search string.
	// If neither IDs nor search is provided then all mailbox messages are updated.
	//
	//	Consumes:
	//	  - application/json
	//
	//	Produces:
	//	  - text/plain
	//
	//	Schemes: http, https
	//
	//	Responses:
	//	  200: OKResponse
	//    400: ErrorResponse

	decoder := json.NewDecoder(r.Body)

	var data struct {
		Read   bool
		IDs    []string
		Search string
	}

	err := decoder.Decode(&data)
	if err != nil {
		httpError(w, err.Error())
		return
	}

	ids := data.IDs
	search := data.Search

	if len(ids) > 0 && search != "" {
		httpError(w, "You may specify either IDs or a search query, not both")
		return
	}

	if search != "" {
		err := storage.SetSearchReadStatus(search, r.URL.Query().Get("tz"), data.Read)
		if err != nil {
			httpError(w, err.Error())
			return
		}
	} else if len(ids) == 0 {
		if data.Read {
			err := storage.MarkAllRead()
			if err != nil {
				httpError(w, err.Error())
				return
			}
		} else {
			err := storage.MarkAllUnread()
			if err != nil {
				httpError(w, err.Error())
				return
			}
		}
	} else {
		if data.Read {
			if err := storage.MarkRead(ids); err != nil {
				httpError(w, err.Error())
				return
			}
		} else {
			if err := storage.MarkUnread(ids); err != nil {
				httpError(w, err.Error())
				return
			}
		}
	}

	w.Header().Add("Content-Type", "text/plain")
	_, _ = w.Write([]byte("ok"))
}

// DeleteMessages (method: DELETE) deletes all messages matching IDS.
func DeleteMessages(w http.ResponseWriter, r *http.Request) {
	// swagger:route DELETE /api/v1/messages messages DeleteMessagesParams
	//
	// # Delete messages
	//
	// Delete individual or all messages. If no IDs are provided then all messages are deleted.
	//
	//	Consumes:
	//	  - application/json
	//
	//	Produces:
	//	  - text/plain
	//
	//	Schemes: http, https
	//
	//	Responses:
	//	  200: OKResponse
	//    400: ErrorResponse

	decoder := json.NewDecoder(r.Body)
	var data struct {
		IDs []string
	}
	err := decoder.Decode(&data)
	if err != nil || len(data.IDs) == 0 {
		if err := storage.DeleteAllMessages(); err != nil {
			httpError(w, err.Error())
			return
		}
	} else {
		if err := storage.DeleteMessages(data.IDs); err != nil {
			httpError(w, err.Error())
			return
		}
	}

	w.Header().Add("Content-Type", "text/plain")
	_, _ = w.Write([]byte("ok"))
}

// Search returns the latest messages as JSON
func Search(w http.ResponseWriter, r *http.Request) {
	// swagger:route GET /api/v1/search messages SearchParams
	//
	// # Search messages
	//
	// Returns messages matching [a search](https://mailpit.axllent.org/docs/usage/search-filters/), sorted by received date (descending).
	//
	//	Produces:
	//	  - application/json
	//
	//	Schemes: http, https
	//
	//	Responses:
	//	  200: MessagesSummaryResponse
	//    400: ErrorResponse

	search := strings.TrimSpace(r.URL.Query().Get("query"))
	if search == "" {
		httpError(w, "Error: no search query")
		return
	}

	start, beforeTS, limit := getStartLimit(r)

	// optionally restrict the search to a single mailbox (authenticated username)
	mailboxName := strings.TrimSpace(r.URL.Query().Get("mailbox"))
	effectiveSearch := search
	if mailboxName != "" {
		effectiveSearch = mailboxSearch(mailboxName) + " " + search
	}

	messages, results, err := storage.Search(effectiveSearch, r.URL.Query().Get("tz"), start, beforeTS, limit)
	if err != nil {
		httpError(w, err.Error())
		return
	}

	stats := storage.StatsGet()
	if mailboxName != "" {
		// totals, unread count & tags are scoped to the mailbox being viewed
		stats = storage.StatsGetForUsername(mailboxName)
	}

	var res MessagesSummary

	res.Start = start
	res.Messages = messages
	res.Count = tools.SafeUint64(len(messages)) // legacy - now undocumented in API specs
	res.Total = stats.Total                     // total messages in mailbox
	res.MessagesCount = tools.SafeUint64(results)
	res.Unread = stats.Unread
	res.Tags = stats.Tags
	res.Usernames = stats.Usernames

	unread, err := storage.SearchUnreadCount(effectiveSearch, r.URL.Query().Get("tz"), beforeTS)
	if err != nil {
		httpError(w, err.Error())
		return
	}

	res.MessagesUnreadCount = tools.SafeUint64(unread)

	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		httpError(w, err.Error())
	}
}

// DeleteSearch will delete all messages matching a search
func DeleteSearch(w http.ResponseWriter, r *http.Request) {
	// swagger:route DELETE /api/v1/search messages DeleteSearchParams
	//
	// # Delete messages by search
	//
	// Delete all messages matching [a search](https://mailpit.axllent.org/docs/usage/search-filters/).
	//
	//	Produces:
	//	  - application/json
	//
	//	Schemes: http, https
	//
	//	Responses:
	//	  200: OKResponse
	//    400: ErrorResponse

	search := strings.TrimSpace(r.URL.Query().Get("query"))
	if search == "" {
		httpError(w, "Error: no search query")
		return
	}

	if err := storage.DeleteSearch(search, r.URL.Query().Get("tz")); err != nil {
		httpError(w, err.Error())
		return
	}

	w.Header().Add("Content-Type", "text/plain")
	_, _ = w.Write([]byte("ok"))
}
