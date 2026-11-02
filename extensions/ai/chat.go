package ai

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/JoaoVitorOli/althea/sdk"
)

// titleLength is how much of the first message names a conversation, in characters.
const titleLength = 60

// conversation is a chat, saved from its first message on unless the user turned that off.
type conversation struct {
	mu       sync.Mutex
	id       string
	saved    bool
	title    string
	messages []Message
	last     string // the latest answer, for the actions
}

func newConversation() *conversation { return &conversation{id: sdk.NewID()} }

// say adds what was said, returning it all so far.
func (c *conversation) say(m Message) []Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, m)
	return append([]Message(nil), c.messages...)
}

func (c *conversation) lastAnswer() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last
}

func (c *conversation) setLast(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.last = text
}

// title names a conversation by its first message.
func titleOf(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	if runes := []rune(line); len(runes) > titleLength {
		return strings.TrimSpace(string(runes[:titleLength])) + "…"
	}
	return line
}

// save adds a message to the conversation's row, making it the first time.
func (e *Extension) save(ctx context.Context, db sdk.Database, c *conversation, m Message) error {
	now := e.Now().UnixMilli()
	c.mu.Lock()
	first := !c.saved
	if first && c.title == "" {
		c.title = titleOf(m.Text)
	}
	title := c.title
	c.saved = true
	c.mu.Unlock()
	return db.Tx(ctx, func(tx sdk.Queryer) error {
		if first {
			if _, err := tx.Exec(ctx,
				`INSERT INTO ai_conversations (id, title, created, updated) VALUES (?, ?, ?, ?)
				 ON CONFLICT (id) DO NOTHING`, c.id, title, now, now); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO ai_messages (conversation, assistant, text, created) VALUES (?, ?, ?, ?)`,
			c.id, m.Assistant, m.Text, now); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE ai_conversations SET updated = ? WHERE id = ?`, now, c.id)
		return err
	})
}

// load reads a conversation's messages.
func load(ctx context.Context, db sdk.Database, id, title string) (*conversation, error) {
	rows, err := db.Query(ctx, `SELECT assistant, text FROM ai_messages WHERE conversation = ? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	c := &conversation{id: id, title: title, saved: true}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.Assistant, &m.Text); err != nil {
			return nil, err
		}
		c.messages = append(c.messages, m)
		if m.Assistant {
			c.last = m.Text
		}
	}
	return c, rows.Err()
}

func remove(ctx context.Context, db sdk.Database, id string) error {
	return db.Tx(ctx, func(tx sdk.Queryer) error {
		if _, err := tx.Exec(ctx, `DELETE FROM ai_messages WHERE conversation = ?`, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM ai_conversations WHERE id = ?`, id)
		return err
	})
}

// conversations lists the saved conversations, the latest first, under a new one.
func (e *Extension) conversations(ctx sdk.Context) *sdk.List {
	return &sdk.List{
		Title:       ctx.T("chat.title"),
		Placeholder: ctx.T("chat.search"),
		Load:        e.conversationItems,
	}
}

func (e *Extension) conversationItems(ctx sdk.Context) ([]sdk.Item, error) {
	items := []sdk.Item{{
		ID: "new", Title: ctx.T("chat.new"), Icon: sdk.Icon{Glyph: "sparkles"},
		Keywords: strings.Fields(ctx.T("chat.new_keywords")),
		Actions: []sdk.Action{{Title: ctx.T("chat.start"), Run: func(ctx sdk.Context) error {
			ctx.Push(e.chatView(ctx, newConversation(), ""))
			return nil
		}}},
	}}
	db := ctx.Database()
	rows, err := db.Query(ctx, `SELECT id, title, updated FROM ai_conversations ORDER BY updated DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	section := ctx.T("chat.saved")
	for rows.Next() {
		var id, title string
		var updated int64
		if err := rows.Scan(&id, &title, &updated); err != nil {
			return nil, err
		}
		items = append(items, sdk.Item{
			ID: id, Title: title, Section: section, Icon: sdk.Icon{Glyph: "document"},
			Accessory: time.UnixMilli(updated).Format(time.DateOnly),
			Actions: []sdk.Action{
				{Title: ctx.T("chat.open"), Run: func(ctx sdk.Context) error {
					c, err := load(ctx, ctx.Database(), id, title)
					if err != nil {
						return err
					}
					ctx.Push(e.chatView(ctx, c, ""))
					return nil
				}},
				{
					Title: ctx.T("chat.delete"), Shortcut: "Ctrl+D", Confirm: ctx.T("chat.delete_confirm"),
					Run: func(ctx sdk.Context) error {
						if err := remove(ctx, ctx.Database(), id); err != nil {
							return err
						}
						ctx.Pop()
						ctx.Push(e.conversations(ctx))
						return nil
					},
				},
			},
		})
	}
	return items, rows.Err()
}

// chatView shows a conversation, saying first when it is set.
func (e *Extension) chatView(ctx sdk.Context, c *conversation, first string) *sdk.Chat {
	said := make([]sdk.ChatMessage, len(c.messages))
	for i, m := range c.messages {
		said[i] = sdk.ChatMessage{Mine: !m.Assistant, Text: m.Text}
	}
	title := c.title
	if title == "" {
		title = titleOf(first)
	}
	if title == "" {
		title = ctx.T("chat.new")
	}
	return &sdk.Chat{
		Title: title, Placeholder: ctx.T("chat.placeholder"), Messages: said, First: first,
		SendTitle: ctx.T("chat.send"),
		Send: func(ctx sdk.Context, text string, show func(string)) error {
			return e.send(ctx, c, text, show)
		},
		Actions: []sdk.Action{
			{Title: ctx.T("chat.copy"), Shortcut: "Ctrl+Shift+C", Run: func(ctx sdk.Context) error {
				if text := c.lastAnswer(); text != "" {
					ctx.Toast(ctx.T("chat.copied"))
					return ctx.CopyText(text)
				}
				return nil
			}},
			{Title: ctx.T("chat.paste"), Shortcut: "Ctrl+Shift+V", Run: func(ctx sdk.Context) error {
				if text := c.lastAnswer(); text != "" {
					return ctx.Paste(sdk.Clip{Text: text})
				}
				return nil
			}},
			{Title: ctx.T("chat.new"), Shortcut: "Ctrl+N", Run: func(ctx sdk.Context) error {
				ctx.Pop()
				ctx.Push(e.chatView(ctx, newConversation(), ""))
				return nil
			}},
		},
	}
}

// send says text in a conversation and writes the answer, keeping what came if it is stopped.
func (e *Extension) send(ctx sdk.Context, c *conversation, text string, show func(string)) error {
	keep := ctx.Preferences().Bool("save_chats")
	user := Message{Text: text}
	all := c.say(user)
	db := ctx.Database()
	saving := func(m Message) {
		if !keep {
			return
		}
		// What came of something stopped is saved too.
		if err := e.save(context.WithoutCancel(ctx), db, c, m); err != nil {
			ctx.Log().Warn("conversation not saved", "err", err)
		}
	}
	saving(user)
	written, err := e.Complete(ctx, Request{Messages: all}, func(s string) { c.setLast(s); show(s) })
	if written != "" {
		m := Message{Assistant: true, Text: written}
		c.say(m)
		c.setLast(written)
		saving(m)
	}
	if ctx.Err() != nil {
		return nil // stopped by the user: what came stays
	}
	return err
}
