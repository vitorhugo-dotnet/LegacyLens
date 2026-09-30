package sqlite

import (
	"context"
	"testing"
	"time"

	"legacylens/core/internal/application"
	"legacylens/core/internal/domain"
)

func TestCommitIndexRollsBackOnFailure(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.SaveProject(ctx, domain.Project{ID: "p", Name: "test", Root: t.TempDir(), CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	first := application.IndexResult{ProjectID: "p", RevisionID: "r1", Artifacts: []domain.Artifact{{ID: "a1", ProjectID: "p", RevisionID: "r1", Path: "view.xhtml", Language: "xhtml", Origin: "filesystem", ContentHash: "old"}}}
	if err := store.CommitIndex(ctx, first); err != nil {
		t.Fatal(err)
	}
	bad := application.IndexResult{ProjectID: "p", RevisionID: "r2", Artifacts: []domain.Artifact{{ID: "a2", ProjectID: "p", RevisionID: "r2", Path: "view.xhtml", Language: "xhtml", Origin: "filesystem", ContentHash: "new"}}, Symbols: []domain.Symbol{{ID: "s", ProjectID: "p", RevisionID: "r2", ArtifactID: "missing", Path: "view.xhtml"}}}
	if err := store.CommitIndex(ctx, bad); err == nil {
		t.Fatal("expected invalid foreign key to fail")
	}
	results, err := store.Search(ctx, application.SearchQuery{ProjectID: "p", RevisionID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Symbols) != 0 {
		t.Fatalf("unexpected result: %#v", results)
	}
	var revision string
	if err := store.db.QueryRow(`SELECT id FROM revisions WHERE project_id = ?`, "p").Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != "r1" {
		t.Fatalf("revision changed after rollback: %s", revision)
	}
	var count int
	if err := store.db.QueryRow(`SELECT count(*) FROM artifacts WHERE revision_id=? AND content_hash=?`, "r1", "old").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("previous artifact was not preserved: count=%d", count)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM revisions WHERE id=?`, "r2").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed revision was partially committed: count=%d", count)
	}
}

func TestListProjectsReturnsRegisteredProjectsInStableOrder(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	created := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	for _, project := range []domain.Project{
		{ID: "p2", Name: "zeta", Root: `C:\zeta`, Includes: []string{"**/*.java"}, Excludes: []string{"target/**"}, CreatedAt: created},
		{ID: "p1", Name: "alpha", Root: `C:\alpha`, CreatedAt: created},
	} {
		if err := store.SaveProject(ctx, project); err != nil {
			t.Fatal(err)
		}
	}
	projects, err := store.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 || projects[0].ID != "p1" || projects[1].ID != "p2" {
		t.Fatalf("ListProjects() order = %+v", projects)
	}
	if len(projects[1].Includes) != 1 || projects[1].Includes[0] != "**/*.java" || len(projects[1].Excludes) != 1 || projects[1].CreatedAt != created {
		t.Fatalf("ListProjects() did not preserve project configuration: %+v", projects[1])
	}
}

func TestProjectAndSearchPagesExposeLaterRowsInStableOrder(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for _, project := range []domain.Project{{ID: "p", Name: "project", Root: t.TempDir(), CreatedAt: time.Now().UTC()}, {ID: "p2", Name: "beta", Root: "/beta"}, {ID: "p3", Name: "zeta", Root: "/zeta"}} {
		if err := store.SaveProject(ctx, project); err != nil {
			t.Fatal(err)
		}
	}
	index := application.IndexResult{ProjectID: "p", RevisionID: "r", Artifacts: []domain.Artifact{{ID: "a", ProjectID: "p", RevisionID: "r", Path: "view.xhtml", Language: "xhtml", Origin: "test"}}}
	for _, id := range []domain.ID{"s3", "s1", "s2"} {
		index.Symbols = append(index.Symbols, domain.Symbol{ID: id, ProjectID: "p", RevisionID: "r", ArtifactID: "a", Path: "view.xhtml", QualifiedName: "save"})
	}
	if err := store.CommitIndex(ctx, index); err != nil {
		t.Fatal(err)
	}
	page, err := store.PageProjects(ctx, 1, 1)
	if err != nil || page.Total != 3 || len(page.Items) != 1 || page.Items[0].ID != "p" || page.Offset != 1 || !page.HasMore {
		t.Fatalf("PageProjects() = %+v, %v", page, err)
	}
	lastProject, err := store.PageProjects(ctx, 2, 1)
	if err != nil || len(lastProject.Items) != 1 || lastProject.Items[0].ID != "p3" || lastProject.HasMore {
		t.Fatalf("last project page = %+v, %v", lastProject, err)
	}
	query := application.SearchQuery{ProjectID: "p", RevisionID: "r", Text: "save", Limit: 1, Offset: 1}
	got, err := store.Search(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil || len(got.Symbols) != 1 || got.Symbols[0].ID != "s2" || got.Total != 3 {
		t.Fatalf("later search page = %+v, %v", got, err)
	}
}
