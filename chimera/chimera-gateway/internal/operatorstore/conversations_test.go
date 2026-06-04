package operatorstore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/testsupport"
)

func openConvTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "operator.sqlite"), testsupport.GatewayOperatorMigrationsDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestConversationHistory_CRUDFlagDelete(t *testing.T) {
	s := openConvTestStore(t)
	ctx := context.Background()
	const (
		principalA = "tenant-a"
		principalB = "tenant-b"
		cid        = "conv-1"
	)

	if err := s.EnsureConversation(ctx, principalA, cid, "Hello from operator chat", ConversationWorkspaceSnapshot{
		ProjectID: "proj", FlavorID: "main",
	}); err != nil {
		t.Fatal(err)
	}

	userTurn, err := s.AppendTurn(ctx, principalA, cid, AppendTurnInput{Role: "user", Content: "Hello from operator chat"})
	if err != nil {
		t.Fatal(err)
	}
	asstTurn, err := s.AppendTurn(ctx, principalA, cid, AppendTurnInput{
		Role: "assistant", Content: "Hi there", SelectedModel: "vm/test", ResolvedModel: "groq/llama",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceTurnRetrievals(ctx, asstTurn, []RetrievalInput{{
		FilePath: "src/a.go", Score: 0.9, SnippetText: "func main() {}", Language: "go",
		VectorPointID: "pt-1", ContentSHA256: "sha256:abc",
		StartLine: 42, EndLine: 58, StartsMidLine: true,
	}}); err != nil {
		t.Fatal(err)
	}
	_ = userTurn

	list, err := s.ListConversations(ctx, principalA, ListConversationsFilter{Limit: 10})
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if list[0].PreviewText == "" || !list[0].Title.Valid {
		t.Fatalf("preview/title not set: %+v", list[0])
	}

	if err := s.SetConversationFlagged(ctx, principalA, cid, true); err != nil {
		t.Fatal(err)
	}
	flagged, err := s.ListConversations(ctx, principalA, ListConversationsFilter{Limit: 10, FlaggedOnly: true})
	if err != nil || len(flagged) != 1 {
		t.Fatalf("flagged list=%+v err=%v", flagged, err)
	}

	if err := s.UpdateConversationTitle(ctx, principalA, cid, "Custom title"); err != nil {
		t.Fatal(err)
	}

	tr, err := s.GetConversationTranscript(ctx, principalA, cid)
	if err != nil || tr == nil || len(tr.Turns) != 2 {
		t.Fatalf("transcript=%+v err=%v", tr, err)
	}
	if len(tr.Turns[1].Retrievals) != 1 {
		t.Fatalf("retrievals=%+v", tr.Turns[1].Retrievals)
	}
	r0 := tr.Turns[1].Retrievals[0]
	if r0.StartLine != 42 || r0.EndLine != 58 || !r0.StartsMidLine {
		t.Fatalf("line metadata=%+v", r0)
	}

	if _, err := s.GetConversationTranscript(ctx, principalB, cid); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetConversationTranscript(ctx, principalB, cid); got != nil {
		t.Fatalf("principal B should not see A conversation: %+v", got)
	}

	if err := s.DeleteConversation(ctx, principalA, cid); err != nil {
		t.Fatal(err)
	}
	after, err := s.ListConversations(ctx, principalA, ListConversationsFilter{Limit: 10})
	if err != nil || len(after) != 0 {
		t.Fatalf("after delete list=%+v err=%v", after, err)
	}
}

func TestConversationHistory_deleteCascade(t *testing.T) {
	s := openConvTestStore(t)
	ctx := context.Background()
	const cid = "conv-cascade"
	if err := s.EnsureConversation(ctx, "p", cid, "x", ConversationWorkspaceSnapshot{}); err != nil {
		t.Fatal(err)
	}
	turnID, err := s.AppendTurn(ctx, "p", cid, AppendTurnInput{Role: "user", Content: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceTurnRetrievals(ctx, turnID, []RetrievalInput{{FilePath: "a.txt", SnippetText: "t"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteConversation(ctx, "p", cid); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversation_turns WHERE conversation_id = ?`, cid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("turns remain: %d", n)
	}
}
