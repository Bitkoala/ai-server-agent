package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ai-server-agent/internal/models"
)

// =============================================================================
// SQLiteStore Tests
// =============================================================================

func TestNewSQLiteStore(t *testing.T) {
	t.Run("in-memory database", func(t *testing.T) {
		store, err := NewSQLiteStore(":memory:")
		if err != nil {
			t.Fatalf("NewSQLiteStore(\":memory:\") failed: %v", err)
		}
		defer store.Close()
		if store.db == nil {
			t.Error("expected non-nil db")
		}
	})

	t.Run("specific path", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "test.db")
		store, err := NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("NewSQLiteStore(%q) failed: %v", dbPath, err)
		}
		defer store.Close()

		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			t.Errorf("expected db file to exist at %q", dbPath)
		}
	})

	t.Run("empty path falls back to default", func(t *testing.T) {
		// When dbPath is "", NewSQLiteStore uses "data/agent.db" as default.
		// The default path may fail in restricted environments (no write access),
		// but we verify the code path doesn't panic and returns a reasonable error.
		store, err := NewSQLiteStore("")
		if err == nil {
			defer store.Close()
			if store.db == nil {
				t.Error("expected non-nil db")
			}
		}
		// Either success or a filesystem error is acceptable for this test
	})
}

func TestSQLiteStore_SaveAndGetTask(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	task := &models.Task{
		ID:        "task-001",
		Intent:    "deploy",
		UserInput: "deploy nginx to production",
		Steps: []models.TaskStep{
			{ID: "step-1", Action: "check_deps", Params: map[string]string{"os": "linux"}, Status: "done", Result: "ok"},
			{ID: "step-2", Action: "install", Params: map[string]string{"pkg": "nginx"}, Status: "pending"},
		},
		Status: "running",
	}

	if err := store.SaveTask(task); err != nil {
		t.Fatalf("SaveTask failed: %v", err)
	}

	got, err := store.GetTask("task-001")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}

	if got.ID != task.ID {
		t.Errorf("ID: got %q, want %q", got.ID, task.ID)
	}
	if got.Intent != task.Intent {
		t.Errorf("Intent: got %q, want %q", got.Intent, task.Intent)
	}
	if got.UserInput != task.UserInput {
		t.Errorf("UserInput: got %q, want %q", got.UserInput, task.UserInput)
	}
	if got.Status != task.Status {
		t.Errorf("Status: got %q, want %q", got.Status, task.Status)
	}
	if len(got.Steps) != len(task.Steps) {
		t.Fatalf("Steps length: got %d, want %d", len(got.Steps), len(task.Steps))
	}
	for i, step := range got.Steps {
		if step.ID != task.Steps[i].ID {
			t.Errorf("Step[%d].ID: got %q, want %q", i, step.ID, task.Steps[i].ID)
		}
		if step.Action != task.Steps[i].Action {
			t.Errorf("Step[%d].Action: got %q, want %q", i, step.Action, task.Steps[i].Action)
		}
		if step.Status != task.Steps[i].Status {
			t.Errorf("Step[%d].Status: got %q, want %q", i, step.Status, task.Steps[i].Status)
		}
		if step.Result != task.Steps[i].Result {
			t.Errorf("Step[%d].Result: got %q, want %q", i, step.Result, task.Steps[i].Result)
		}
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestSQLiteStore_UpdateTask(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	task := &models.Task{
		ID:        "task-002",
		Intent:    "backup",
		UserInput: "backup database",
		Steps: []models.TaskStep{
			{ID: "step-1", Action: "dump", Status: "pending"},
		},
		Status: "pending",
	}

	if err := store.SaveTask(task); err != nil {
		t.Fatalf("SaveTask failed: %v", err)
	}

	// Update
	task.Status = "done"
	task.Steps = []models.TaskStep{
		{ID: "step-1", Action: "dump", Status: "done", Result: "backup completed"},
	}
	if err := store.UpdateTask(task); err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	got, err := store.GetTask("task-002")
	if err != nil {
		t.Fatalf("GetTask after update failed: %v", err)
	}

	if got.Status != "done" {
		t.Errorf("Status after update: got %q, want %q", got.Status, "done")
	}
	if got.Steps[0].Status != "done" {
		t.Errorf("Step status after update: got %q, want %q", got.Steps[0].Status, "done")
	}
	if got.Steps[0].Result != "backup completed" {
		t.Errorf("Step result after update: got %q, want %q", got.Steps[0].Result, "backup completed")
	}
}

func TestSQLiteStore_GetTask_NotFound(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	_, err := store.GetTask("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent task, got nil")
	}
}

func TestSQLiteStore_ListTasks(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// Save multiple tasks with staggered creation times
	tasks := []*models.Task{
		{ID: "list-1", Intent: "deploy", UserInput: "deploy app", Steps: []models.TaskStep{{ID: "s1", Action: "build", Status: "done"}}, Status: "done"},
		{ID: "list-2", Intent: "monitor", UserInput: "check health", Steps: []models.TaskStep{{ID: "s1", Action: "ping", Status: "done"}}, Status: "done"},
		{ID: "list-3", Intent: "backup", UserInput: "backup data", Steps: []models.TaskStep{{ID: "s1", Action: "dump", Status: "pending"}}, Status: "pending"},
	}

	for _, task := range tasks {
		if err := store.SaveTask(task); err != nil {
			t.Fatalf("SaveTask %q failed: %v", task.ID, err)
		}
		time.Sleep(10 * time.Millisecond) // ensure distinct created_at
	}

	t.Run("list all", func(t *testing.T) {
		got, err := store.ListTasks(10)
		if err != nil {
			t.Fatalf("ListTasks failed: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("expected 3 tasks, got %d", len(got))
		}
		// DESC by created_at: newest first (list-3, list-2, list-1)
		if got[0].ID != "list-3" {
			t.Errorf("first task: got %q, want list-3", got[0].ID)
		}
		if got[1].ID != "list-2" {
			t.Errorf("second task: got %q, want list-2", got[1].ID)
		}
		if got[2].ID != "list-1" {
			t.Errorf("third task: got %q, want list-1", got[2].ID)
		}
	})

	t.Run("list with limit", func(t *testing.T) {
		got, err := store.ListTasks(2)
		if err != nil {
			t.Fatalf("ListTasks with limit failed: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 tasks, got %d", len(got))
		}
		if got[0].ID != "list-3" {
			t.Errorf("first task: got %q, want list-3", got[0].ID)
		}
		if got[1].ID != "list-2" {
			t.Errorf("second task: got %q, want list-2", got[1].ID)
		}
	})
}

func TestSQLiteStore_Close(t *testing.T) {
	store := newTestStore(t)

	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Double close should not panic (sql.DB handles this gracefully)
	if err := store.Close(); err != nil {
		t.Logf("second Close returned: %v (expected)", err)
	}
}

// =============================================================================
// AuditLogger Tests
// =============================================================================

func TestNewAuditLogger(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	al, err := NewAuditLogger(store)
	if err != nil {
		t.Fatalf("NewAuditLogger failed: %v", err)
	}
	if al == nil {
		t.Fatal("expected non-nil AuditLogger")
	}
	if al.store != store {
		t.Error("store reference mismatch")
	}
	if al.lastHash != "" {
		t.Logf("initial lastHash on empty DB: %q (expected empty)", al.lastHash)
	}
}

func TestAuditLogger_Log(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	al, err := NewAuditLogger(store)
	if err != nil {
		t.Fatalf("NewAuditLogger failed: %v", err)
	}

	entry := &AuditEntry{
		TaskID:     "task-1",
		StepID:     "step-1",
		Action:     "execute",
		Result:     "success",
		UserID:     "user-1",
		DurationMs: 150,
	}

	if err := al.Log(entry); err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Verify the entry was persisted
	entries, err := al.Query("task-1", 10)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	got := entries[0]
	if got.TaskID != "task-1" {
		t.Errorf("TaskID: got %q, want %q", got.TaskID, "task-1")
	}
	if got.StepID != "step-1" {
		t.Errorf("StepID: got %q, want %q", got.StepID, "step-1")
	}
	if got.Action != "execute" {
		t.Errorf("Action: got %q, want %q", got.Action, "execute")
	}
	if got.Result != "success" {
		t.Errorf("Result: got %q, want %q", got.Result, "success")
	}
	if got.UserID != "user-1" {
		t.Errorf("UserID: got %q, want %q", got.UserID, "user-1")
	}
	if got.DurationMs != 150 {
		t.Errorf("DurationMs: got %d, want 150", got.DurationMs)
	}
	if got.ChainHash == "" {
		t.Error("ChainHash should not be empty")
	}
}

func TestAuditLogger_LogOperation(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	al, err := NewAuditLogger(store)
	if err != nil {
		t.Fatalf("NewAuditLogger failed: %v", err)
	}

	params := map[string]string{"host": "server01", "port": "8080"}
	al.LogOperation("task-op", "step-op", "ssh_connect", params, "connected successfully", nil, 250)

	entries, err := al.Query("task-op", 10)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	got := entries[0]
	if got.TaskID != "task-op" {
		t.Errorf("TaskID: got %q, want task-op", got.TaskID)
	}
	if got.Action != "ssh_connect" {
		t.Errorf("Action: got %q, want ssh_connect", got.Action)
	}
	if got.DurationMs != 250 {
		t.Errorf("DurationMs: got %d, want 250", got.DurationMs)
	}
	if got.Error != "" {
		t.Errorf("Error: expected empty, got %q", got.Error)
	}
	// ParamsBefore should be a JSON string of the params map
	var parsed map[string]string
	if err := json.Unmarshal([]byte(got.ParamsBefore), &parsed); err != nil {
		t.Errorf("ParamsBefore is not valid JSON: %v", err)
	}
	if parsed["host"] != "server01" {
		t.Errorf("ParamsBefore host: got %q, want server01", parsed["host"])
	}
}

func TestAuditLogger_LogDiff(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	al, err := NewAuditLogger(store)
	if err != nil {
		t.Fatalf("NewAuditLogger failed: %v", err)
	}

	before := map[string]string{"version": "1.0", "replicas": "1"}
	after := map[string]string{"version": "2.0", "replicas": "3"}
	al.LogDiff("task-diff", "step-diff", "scale", before, after, "scaled successfully", nil, 500)

	entries, err := al.Query("task-diff", 10)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	got := entries[0]
	if got.TaskID != "task-diff" {
		t.Errorf("TaskID: got %q, want task-diff", got.TaskID)
	}
	if got.Action != "scale" {
		t.Errorf("Action: got %q, want scale", got.Action)
	}

	var parsedBefore, parsedAfter map[string]string
	if err := json.Unmarshal([]byte(got.ParamsBefore), &parsedBefore); err != nil {
		t.Errorf("ParamsBefore is not valid JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(got.ParamsAfter), &parsedAfter); err != nil {
		t.Errorf("ParamsAfter is not valid JSON: %v", err)
	}
	if parsedBefore["version"] != "1.0" {
		t.Errorf("ParamsBefore version: got %q, want 1.0", parsedBefore["version"])
	}
	if parsedAfter["version"] != "2.0" {
		t.Errorf("ParamsAfter version: got %q, want 2.0", parsedAfter["version"])
	}
}

func TestAuditLogger_Verify(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	al, err := NewAuditLogger(store)
	if err != nil {
		t.Fatalf("NewAuditLogger failed: %v", err)
	}

	// Add multiple entries to build a chain
	for i := 0; i < 5; i++ {
		entry := &AuditEntry{
			TaskID: "verify-task",
			Action: "test_action",
			Result: "ok",
		}
		if err := al.Log(entry); err != nil {
			t.Fatalf("Log #%d failed: %v", i, err)
		}
	}

	valid, tampered, err := al.Verify()
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if !valid {
		t.Errorf("chain should be valid, tampered entries: %v", tampered)
	}
	if len(tampered) != 0 {
		t.Errorf("expected no tampered entries, got %d", len(tampered))
	}
}

func TestAuditLogger_Query(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	al, err := NewAuditLogger(store)
	if err != nil {
		t.Fatalf("NewAuditLogger failed: %v", err)
	}

	// Add entries for different tasks
	al.Log(&AuditEntry{TaskID: "q-task-a", Action: "action-a", Result: "ok"})
	al.Log(&AuditEntry{TaskID: "q-task-a", Action: "action-b", Result: "ok"})
	al.Log(&AuditEntry{TaskID: "q-task-b", Action: "action-c", Result: "ok"})

	t.Run("query by taskID", func(t *testing.T) {
		entries, err := al.Query("q-task-a", 10)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries for q-task-a, got %d", len(entries))
		}
		for _, e := range entries {
			if e.TaskID != "q-task-a" {
				t.Errorf("unexpected TaskID: %q", e.TaskID)
			}
		}
	})

	t.Run("query without taskID", func(t *testing.T) {
		entries, err := al.Query("", 10)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(entries) != 3 {
			t.Fatalf("expected 3 entries total, got %d", len(entries))
		}
	})

	t.Run("query with limit", func(t *testing.T) {
		entries, err := al.Query("", 1)
		if err != nil {
			t.Fatalf("Query with limit failed: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry with limit, got %d", len(entries))
		}
	})
}

func TestAuditLogger_ExportJSON(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	al, err := NewAuditLogger(store)
	if err != nil {
		t.Fatalf("NewAuditLogger failed: %v", err)
	}

	al.Log(&AuditEntry{TaskID: "export-task", Action: "test", Result: "export test", UserID: "user-1"})

	data, err := al.ExportJSON("export-task")
	if err != nil {
		t.Fatalf("ExportJSON failed: %v", err)
	}

	var entries []AuditEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("exported data is not valid JSON: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry in export, got %d", len(entries))
	}
	if entries[0].TaskID != "export-task" {
		t.Errorf("TaskID: got %q, want export-task", entries[0].TaskID)
	}
}

func TestAuditLogger_TamperDetection(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	al, err := NewAuditLogger(store)
	if err != nil {
		t.Fatalf("NewAuditLogger failed: %v", err)
	}

	// Add valid entries
	al.Log(&AuditEntry{TaskID: "tamper-task", Action: "step-1", Result: "ok"})
	al.Log(&AuditEntry{TaskID: "tamper-task", Action: "step-2", Result: "ok"})
	al.Log(&AuditEntry{TaskID: "tamper-task", Action: "step-3", Result: "ok"})

	// Verify chain is valid before tampering
	valid, tampered, err := al.Verify()
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if !valid {
		t.Fatalf("chain should be valid before tampering, got: %v", tampered)
	}
	if len(tampered) != 0 {
		t.Fatalf("expected 0 tampered entries, got %d", len(tampered))
	}

	// Tamper with a record directly in the DB
	_, err = store.db.Exec(`UPDATE audit_logs SET prev_hash = 'tampered-hash' WHERE id = 2`)
	if err != nil {
		t.Fatalf("failed to tamper: %v", err)
	}

	// Now verify should detect tampering
	valid, tampered, err = al.Verify()
	if err != nil {
		t.Fatalf("Verify after tamper failed: %v", err)
	}
	if valid {
		t.Error("chain should be invalid after tampering")
	}
	if len(tampered) == 0 {
		t.Error("expected tampered entries to be reported")
	}
}

// =============================================================================
// JSON Helpers Tests
// =============================================================================

func TestMustMarshal(t *testing.T) {
	t.Run("valid data", func(t *testing.T) {
		data := map[string]string{"key": "value", "foo": "bar"}
		result := mustMarshal(data)
		if result == "" {
			t.Error("expected non-empty JSON string")
		}
		if result == "[]" {
			t.Error("expected valid JSON, not fallback")
		}

		var parsed map[string]string
		if err := json.Unmarshal([]byte(result), &parsed); err != nil {
			t.Errorf("result is not valid JSON: %v", err)
		}
		if parsed["key"] != "value" {
			t.Errorf("key: got %q, want value", parsed["key"])
		}
	})

	t.Run("invalid data returns fallback", func(t *testing.T) {
		// channels cannot be marshaled
		result := mustMarshal(make(chan int))
		if result != "[]" {
			t.Errorf("expected fallback \"[]\", got %q", result)
		}
	})

	t.Run("nil data", func(t *testing.T) {
		result := mustMarshal(nil)
		if result != "null" {
			t.Errorf("expected \"null\", got %q", result)
		}
	})

	t.Run("slice data", func(t *testing.T) {
		data := []int{1, 2, 3}
		result := mustMarshal(data)
		var parsed []int
		if err := json.Unmarshal([]byte(result), &parsed); err != nil {
			t.Errorf("result is not valid JSON: %v", err)
		}
		if len(parsed) != 3 {
			t.Errorf("expected 3 elements, got %d", len(parsed))
		}
	})

	t.Run("string data", func(t *testing.T) {
		result := mustMarshal("hello")
		if result != `"hello"` {
			t.Errorf("expected %q, got %q", `"hello"`, result)
		}
	})
}

func TestMustUnmarshal(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		var data map[string]string
		err := mustUnmarshal(`{"name":"test","value":"123"}`, &data)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if data["name"] != "test" {
			t.Errorf("name: got %q, want test", data["name"])
		}
		if data["value"] != "123" {
			t.Errorf("value: got %q, want 123", data["value"])
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		var data map[string]string
		err := mustUnmarshal(`{invalid json`, &data)
		if err == nil {
			t.Error("expected error for invalid JSON, got nil")
		}
	})

	t.Run("empty JSON object", func(t *testing.T) {
		var data map[string]string
		err := mustUnmarshal(`{}`, &data)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if len(data) != 0 {
			t.Errorf("expected empty map, got %d entries", len(data))
		}
	})

	t.Run("empty string", func(t *testing.T) {
		var data map[string]string
		err := mustUnmarshal("", &data)
		if err == nil {
			t.Error("expected error for empty string, got nil")
		}
	})

	t.Run("unmarshal into struct", func(t *testing.T) {
		type testStruct struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}
		var s testStruct
		err := mustUnmarshal(`{"name":"Alice","age":30}`, &s)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if s.Name != "Alice" {
			t.Errorf("Name: got %q, want Alice", s.Name)
		}
		if s.Age != 30 {
			t.Errorf("Age: got %d, want 30", s.Age)
		}
	})
}

// =============================================================================
// Helpers
// =============================================================================

// newTestStore creates a SQLiteStore backed by an in-memory database.
func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory store: %v", err)
	}
	return store
}
