package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestImageTaskServiceIdempotencyOwnerIsolationAndCompletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image_tasks.json")
	handlerCalls := make(chan map[string]any, 4)
	handler := func(ctx context.Context, identity Identity, payload map[string]any) (map[string]any, error) {
		handlerCalls <- payload
		return map[string]any{"data": []map[string]any{{"url": "https://example.test/image.png"}}}, nil
	}
	svc := NewImageTaskService(path, handler, handler, func() int { return 30 })

	alice := Identity{ID: "alice", Name: "Alice", Role: "user"}
	bob := Identity{ID: "bob", Name: "Bob", Role: "user"}

	first, err := svc.SubmitGeneration(context.Background(), alice, "task-1", "draw", "gpt-image-2", "1024x1024", "https://base.test", 1)
	if err != nil {
		t.Fatalf("SubmitGeneration() error = %v", err)
	}
	second, err := svc.SubmitGeneration(context.Background(), alice, "task-1", "different", "gpt-image-2", "1024x1024", "https://base.test", 1)
	if err != nil {
		t.Fatalf("second SubmitGeneration() error = %v", err)
	}
	if first["id"] != second["id"] {
		t.Fatalf("idempotent task id mismatch: %#v %#v", first, second)
	}
	waitForTaskStatus(t, svc, alice, "task-1", TaskStatusSuccess)
	select {
	case <-handlerCalls:
	default:
		t.Fatal("handler was not called")
	}
	if len(handlerCalls) != 0 {
		t.Fatalf("handler calls after duplicate = %d extra, want 0", len(handlerCalls))
	}
	if got := svc.ListTasks(bob, []string{"task-1"}); len(got["items"].([]map[string]any)) != 0 {
		t.Fatalf("bob can see alice task: %#v", got)
	}
	if got := svc.ListTasks(bob, []string{"task-1"}); len(got["missing_ids"].([]string)) != 1 {
		t.Fatalf("bob missing ids = %#v", got)
	}
}

func TestImageTaskServiceListTasksReturnsEmptyArrays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image_tasks.json")
	svc := NewImageTaskService(path, failingImageTaskHandler, failingImageTaskHandler, func() int { return 30 })
	identity := Identity{ID: "alice", Name: "Alice", Role: "user"}

	for name, got := range map[string]map[string]any{
		"empty list":   svc.ListTasks(identity, nil),
		"missing task": svc.ListTasks(identity, []string{"missing"}),
	} {
		items, ok := got["items"].([]map[string]any)
		if !ok {
			t.Fatalf("%s items type = %T", name, got["items"])
		}
		if items == nil {
			t.Fatalf("%s items is nil", name)
		}
		missing, ok := got["missing_ids"].([]string)
		if !ok {
			t.Fatalf("%s missing_ids type = %T", name, got["missing_ids"])
		}
		if missing == nil {
			t.Fatalf("%s missing_ids is nil", name)
		}

		data, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("%s Marshal() error = %v", name, err)
		}
		text := string(data)
		if strings.Contains(text, `"items":null`) || strings.Contains(text, `"missing_ids":null`) {
			t.Fatalf("%s encoded nil arrays: %s", name, text)
		}
	}
}

func TestImageTaskServiceLimitsConcurrentImageSlots(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image_tasks.json")
	started := make(chan string, 2)
	release := make(chan struct{})
	handler := func(ctx context.Context, identity Identity, payload map[string]any) (map[string]any, error) {
		started <- payload["prompt"].(string)
		<-release
		return map[string]any{"data": []map[string]any{{"url": "https://example.test/image.png"}}}, nil
	}
	svc := NewImageTaskService(path, handler, handler, func() int { return 30 }, func() int { return 2 })
	identity := Identity{ID: "alice", Name: "Alice", Role: "user"}

	if _, err := svc.SubmitGeneration(context.Background(), identity, "task-1", "first", "gpt-image-2", "1024x1024", "https://base.test", 2); err != nil {
		t.Fatalf("SubmitGeneration(first) error = %v", err)
	}
	if got := waitForStartedTask(t, started); got != "first" {
		t.Fatalf("started task = %q, want first", got)
	}
	if _, err := svc.SubmitGeneration(context.Background(), identity, "task-2", "second", "gpt-image-2", "1024x1024", "https://base.test", 1); err != nil {
		t.Fatalf("SubmitGeneration(second) error = %v", err)
	}

	time.Sleep(350 * time.Millisecond)
	if got := svc.ListTasks(identity, []string{"task-2"}); got["items"].([]map[string]any)[0]["status"] != TaskStatusQueued {
		t.Fatalf("second task should stay queued while first holds image slots: %#v", got)
	}
	select {
	case prompt := <-started:
		t.Fatalf("second task started before slots were released: %s", prompt)
	default:
	}

	close(release)
	if got := waitForStartedTask(t, started); got != "second" {
		t.Fatalf("started task after release = %q, want second", got)
	}
	waitForTaskStatus(t, svc, identity, "task-1", TaskStatusSuccess)
	waitForTaskStatus(t, svc, identity, "task-2", TaskStatusSuccess)
}

func TestImageTaskServiceCancelsQueuedTask(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image_tasks.json")
	started := make(chan string, 2)
	release := make(chan struct{})
	handler := func(ctx context.Context, identity Identity, payload map[string]any) (map[string]any, error) {
		started <- payload["prompt"].(string)
		<-release
		return map[string]any{"data": []map[string]any{{"url": "https://example.test/image.png"}}}, nil
	}
	svc := NewImageTaskService(path, handler, handler, func() int { return 30 }, func() int { return 1 })
	identity := Identity{ID: "alice", Name: "Alice", Role: "user"}

	if _, err := svc.SubmitGeneration(context.Background(), identity, "task-1", "first", "gpt-image-2", "1024x1024", "https://base.test", 1); err != nil {
		t.Fatalf("SubmitGeneration(first) error = %v", err)
	}
	if got := waitForStartedTask(t, started); got != "first" {
		t.Fatalf("started task = %q, want first", got)
	}
	if _, err := svc.SubmitGeneration(context.Background(), identity, "task-2", "second", "gpt-image-2", "1024x1024", "https://base.test", 1); err != nil {
		t.Fatalf("SubmitGeneration(second) error = %v", err)
	}

	cancelled, err := svc.CancelTask(identity, "task-2")
	if err != nil {
		t.Fatalf("CancelTask() error = %v", err)
	}
	if cancelled["status"] != TaskStatusCancelled {
		t.Fatalf("cancelled task status = %#v", cancelled)
	}
	close(release)
	waitForTaskStatus(t, svc, identity, "task-1", TaskStatusSuccess)
	waitForTaskStatus(t, svc, identity, "task-2", TaskStatusCancelled)
	select {
	case prompt := <-started:
		t.Fatalf("cancelled queued task still started: %s", prompt)
	default:
	}
}

func TestImageTaskServiceCancelsRunningTask(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image_tasks.json")
	started := make(chan struct{})
	handlerDone := make(chan error, 1)
	handler := func(ctx context.Context, identity Identity, payload map[string]any) (map[string]any, error) {
		close(started)
		<-ctx.Done()
		handlerDone <- ctx.Err()
		return nil, ctx.Err()
	}
	svc := NewImageTaskService(path, handler, handler, func() int { return 30 })
	identity := Identity{ID: "alice", Name: "Alice", Role: "user"}

	if _, err := svc.SubmitGeneration(context.Background(), identity, "task-1", "draw", "gpt-image-2", "1024x1024", "https://base.test", 1); err != nil {
		t.Fatalf("SubmitGeneration() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task handler to start")
	}

	cancelled, err := svc.CancelTask(identity, "task-1")
	if err != nil {
		t.Fatalf("CancelTask() error = %v", err)
	}
	if cancelled["status"] != TaskStatusCancelled {
		t.Fatalf("cancelled task status = %#v", cancelled)
	}
	select {
	case err := <-handlerDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("handler ctx err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("task handler did not observe cancellation")
	}
	waitForTaskStatus(t, svc, identity, "task-1", TaskStatusCancelled)
}

func TestImageTaskServicePreservesPartialDataOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image_tasks.json")
	handler := func(ctx context.Context, identity Identity, payload map[string]any) (map[string]any, error) {
		return map[string]any{"data": []map[string]any{{"url": "https://example.test/first.png"}}}, errors.New("second image failed")
	}
	svc := NewImageTaskService(path, handler, handler, func() int { return 30 })
	identity := Identity{ID: "alice", Name: "Alice", Role: "user"}

	if _, err := svc.SubmitGeneration(context.Background(), identity, "task-1", "draw", "gpt-image-2", "1024x1024", "https://base.test", 2); err != nil {
		t.Fatalf("SubmitGeneration() error = %v", err)
	}
	waitForTaskStatus(t, svc, identity, "task-1", TaskStatusError)
	got := svc.ListTasks(identity, []string{"task-1"})
	item := got["items"].([]map[string]any)[0]
	data := item["data"].([]map[string]any)
	if len(data) != 1 || data[0]["url"] != "https://example.test/first.png" {
		t.Fatalf("partial data was not preserved: %#v", item)
	}
	if item["error"] != "second image failed" {
		t.Fatalf("partial failure error = %#v", item)
	}
}

func TestImageTaskServiceRestoresUnfinishedTasksAsErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image_tasks.json")
	raw := map[string]any{"tasks": []map[string]any{
		{"id": "queued", "owner_id": "alice", "status": TaskStatusQueued, "mode": "generate", "created_at": "2026-01-01 00:00:00", "updated_at": "2026-01-01 00:00:00"},
		{"id": "running", "owner_id": "alice", "status": TaskStatusRunning, "mode": "edit", "created_at": "2026-01-01 00:00:00", "updated_at": "2026-01-01 00:00:00"},
	}}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := NewImageTaskService(path, failingImageTaskHandler, failingImageTaskHandler, func() int { return 30 })
	got := svc.ListTasks(Identity{ID: "alice"}, []string{"queued", "running"})
	items := got["items"].([]map[string]any)
	if len(items) != 2 {
		t.Fatalf("items = %#v", items)
	}
	for _, item := range items {
		if item["status"] != TaskStatusError {
			t.Fatalf("unfinished task was not restored as error: %#v", item)
		}
		if item["error"] == nil {
			t.Fatalf("restored task missing error text: %#v", item)
		}
	}
}

func waitForStartedTask(t *testing.T, started <-chan string) string {
	t.Helper()
	select {
	case prompt := <-started:
		return prompt
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task handler to start")
	}
	return ""
}

func failingImageTaskHandler(context.Context, Identity, map[string]any) (map[string]any, error) {
	return nil, errors.New("unexpected handler call")
}

func waitForTaskStatus(t *testing.T, svc *ImageTaskService, identity Identity, taskID, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got := svc.ListTasks(identity, []string{taskID})
		items := got["items"].([]map[string]any)
		if len(items) == 1 && items[0]["status"] == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("task %s did not reach status %s", taskID, want)
}
