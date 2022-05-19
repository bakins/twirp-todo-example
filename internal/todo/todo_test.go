package todo_test

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bakins/twirp-todo-example/internal/database"
	pb "github.com/bakins/twirp-todo-example/internal/proto"
	"github.com/bakins/twirp-todo-example/internal/todo"
)

func TestServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	defer func() {
		walk := func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !strings.Contains(path, "testing.db") {
				return nil
			}

			err = os.Remove(path)
			assert.NoError(t, err)

			return nil
		}

		err := filepath.Walk("data", walk)
		assert.NoError(t, err)
	}()

	cwd, err := os.Getwd()
	require.NoError(t, err)

	dbfile := filepath.Join("data", "testing.db")

	cfg := database.Config{
		SchemaDirectory: filepath.Join(filepath.Dir(filepath.Dir(cwd)), "schema"),
		Filename:        dbfile,
	}

	db, err := cfg.Build(ctx)
	require.NoError(t, err)

	defer db.Close()

	s, err := todo.New(db)
	require.NoError(t, err)

	defer s.Close()

	svr := httptest.NewServer(pb.NewTodoServiceServer(s))
	defer svr.Close()

	client := pb.NewTodoServiceProtobufClient(svr.URL, http.DefaultClient)

	t.Run("create task", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			resp, err := client.CreateTask(
				ctx,
				&pb.CreateTaskRequest{
					Title: "testing",
				},
			)

			require.NoError(t, err)

			require.Equal(t, "testing", resp.Task.Title)
			require.Equal(t, uint64(i+1), resp.Task.Id)
		}
	})

	t.Run("list tasks", func(t *testing.T) {
		resp, err := client.ListTasks(
			ctx,
			&pb.ListTasksRequest{},
		)

		require.NoError(t, err)

		require.Len(t, resp.Tasks, 10)

		for i := 0; i < 10; i++ {
			require.Equal(t, "testing", resp.Tasks[i].Title)
			require.Equal(t, uint64(i+1), resp.Tasks[i].Id)
		}
	})

	t.Run("get task", func(t *testing.T) {
		resp, err := client.GetTask(
			ctx,
			&pb.GetTaskRequest{
				Id: 1,
			},
		)

		require.NoError(t, err)

		require.Equal(t, "testing", resp.Task.Title)
		require.Equal(t, uint64(1), resp.Task.Id)
	})
}

func BenchmarkServer(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cleanup := func() {
		walk := func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !strings.Contains(path, "testing.db") {
				return nil
			}

			err = os.Remove(path)
			assert.NoError(b, err)

			return nil
		}
		err := filepath.Walk("data", walk)
		assert.NoError(b, err)
	}

	b.Cleanup(cleanup)

	cwd, err := os.Getwd()
	require.NoError(b, err)

	dbfile := filepath.Join("data", "testing.db")

	cfg := database.Config{
		SchemaDirectory: filepath.Join(filepath.Dir(filepath.Dir(cwd)), "schema"),
		Filename:        dbfile,
	}

	db, err := cfg.Build(ctx)
	require.NoError(b, err)

	b.Cleanup(func() { _ = db.Close() })

	s, err := todo.New(db)
	require.NoError(b, err)

	b.Cleanup(func() { s.Close() })

	_, err = s.CreateTask(
		ctx,
		&pb.CreateTaskRequest{
			Title: "testing",
		},
	)

	require.NoError(b, err)

	b.ResetTimer()

	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			_, err := s.GetTask(
				ctx,
				&pb.GetTaskRequest{
					Id: 1,
				},
			)
			if err != nil {
				b.Error(err)
			}
		}
	})
}
