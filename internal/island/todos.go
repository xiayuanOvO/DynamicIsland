package island

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

const noTodoReminder = -1

var todoLeadMinutes = []int{noTodoReminder, 0, 5, 15, 30, 60}

type todoItem struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Due         time.Time `json:"due,omitzero"`
	LeadMinutes int       `json:"leadMinutes"`
	Done        bool      `json:"done,omitempty"`
	Notified    bool      `json:"notified,omitempty"`
}

func (item todoItem) reminderDue(now time.Time) bool {
	return !item.Done && !item.Notified && !item.Due.IsZero() && item.LeadMinutes != noTodoReminder &&
		!now.Before(item.Due.Add(-time.Duration(item.LeadMinutes)*time.Minute))
}

type todoComponent struct {
	items      []todoItem
	open       bool
	draft      *todoDraft
	leaving    *todoDraft
	loadFailed bool
	problem    string
	delivering map[string]context.CancelFunc
}

func (t *todoComponent) next() *todoItem {
	if len(t.items) == 0 || t.items[0].Done {
		return nil
	}
	return &t.items[0]
}

func sortTodos(items []todoItem) {
	slices.SortStableFunc(items, func(a, b todoItem) int {
		switch {
		case a.Done != b.Done:
			if a.Done {
				return 1
			}
			return -1
		case a.Due.IsZero() != b.Due.IsZero():
			if a.Due.IsZero() {
				return 1
			}
			return -1
		default:
			return a.Due.Compare(b.Due)
		}
	})
}

func (a *island) todosPath() string {
	return filepath.Join(filepath.Dir(a.statePath), "todos.json")
}

func (a *island) loadTodos() {
	var items []todoItem
	data, err := os.ReadFile(a.todosPath())
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	} else if err == nil {
		err = json.Unmarshal(data, &items)
		if err == nil {
			seen := make(map[string]bool, len(items))
			for i, item := range items {
				if item.ID == "" || seen[item.ID] || strings.TrimSpace(item.Title) == "" || !slices.Contains(todoLeadMinutes, item.LeadMinutes) {
					err = fmt.Errorf("无效的待办事项 %d", i+1)
					break
				}
				seen[item.ID] = true
				if !item.Due.IsZero() {
					items[i].Due = item.Due.Local()
				}
			}
		}
	}
	a.todos.loadFailed = err != nil
	if err != nil {
		log.Printf("load todos: %v", err)
		a.todos.problem = "待办读取失败"
		return
	}
	sortTodos(items)
	a.todos.items, a.todos.problem = items, ""
}

// Commit the list before updating the panel or canceling a pending reminder.
func (a *island) changeTodo(item todoItem, remove bool) bool {
	t := &a.todos
	index := slices.IndexFunc(t.items, func(other todoItem) bool { return other.ID == item.ID })
	items := slices.Clone(t.items)
	switch {
	case remove:
		items = slices.Delete(items, index, index+1)
	case index < 0:
		items = append(items, item)
	default:
		previous := items[index]
		item.Notified = previous.Notified && previous.Due.Equal(item.Due) && previous.LeadMinutes == item.LeadMinutes
		items[index] = item
	}
	sortTodos(items)
	if err := saveJSON(a.todosPath(), items); err != nil {
		log.Printf("save todos: %v", err)
		t.problem = "待办保存失败"
		return false
	}
	t.items, t.problem = items, ""
	if cancel := t.delivering[item.ID]; cancel != nil {
		cancel()
		delete(t.delivering, item.ID)
	}
	if a.win != nil {
		a.win.Invalidate()
	}
	return true
}

// Delivery finishes on the UI thread, where edits and the delivery record are serialized.
func (a *island) checkTodoReminders(now time.Time, deliver func(context.Context, todoItem, func(error))) {
	t := &a.todos
	for _, item := range t.items {
		if !item.reminderDue(now) || t.delivering[item.ID] != nil {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		t.delivering[item.ID] = cancel
		deliver(ctx, item, func(err error) {
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				// Retry on an edit or restart without repeatedly requesting permission.
				log.Printf("notify todo: %v", err)
				t.problem = "系统通知未送达 · " + item.Title
				if a.win != nil {
					a.win.Invalidate()
					if a.win.IsVisible() {
						a.setExpanded(true)
					}
				}
			} else {
				delete(t.delivering, item.ID)
				cancel()
				for i := range t.items {
					if t.items[i].ID == item.ID {
						t.items[i].Notified = true
						break
					}
				}
				if err := saveJSON(a.todosPath(), t.items); err != nil {
					log.Printf("save todo reminder: %v", err)
					t.problem = "提醒记录保存失败"
					if a.win != nil {
						a.win.Invalidate()
					}
				}
			}
		})
	}
}

func (a *island) watchTodoReminders() {
	t := &a.todos
	t.delivering = make(map[string]context.CancelFunc)
	done := make(chan struct{})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		close(done)
		for _, cancel := range t.delivering {
			cancel()
		}
	})
	check := func() {
		a.checkTodoReminders(time.Now(), func(ctx context.Context, item todoItem, finish func(error)) {
			notice := mygo.NewNotification(mygo.NotificationOptions{
				ID: "todo-" + item.ID, Group: "todos", Title: item.Title,
				Body: "截止时间 " + item.Due.Format("2006-01-02 15:04"),
			})
			go func() {
				err := notice.ShowContext(ctx)
				mygo.RunOnMain(func() { finish(err) })
			}()
		})
	}
	check()
	go func() {
		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				mygo.RunOnMain(check)
			case <-done:
				return
			}
		}
	}()
}
