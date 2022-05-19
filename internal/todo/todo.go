package todo

import (
	"context"
	"database/sql"
	"time"

	"github.com/twitchtv/twirp"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/bakins/twirp-todo-example/internal/proto"
)

type Server struct {
	db        *sql.DB
	stmtCache *stmtCache
}

var _ pb.TodoService = &Server{}

func New(db *sql.DB) (*Server, error) {
	s := Server{
		db:        db,
		stmtCache: newStmtCache(db),
	}

	return &s, nil
}

func (s *Server) Close() {
	s.stmtCache.Close()
}

func (s *Server) ListTasks(ctx context.Context, req *pb.ListTasksRequest) (*pb.ListTasksResponse, error) {
	rows, err := s.stmtCache.QueryContext(ctx,
		"select id, created, title, description from tasks order by id",
	)
	if err != nil {
		// TODO: map sql error to more fitting twirp error
		return nil, twirp.InternalErrorWith(err)
	}
	defer rows.Close()

	var resp pb.ListTasksResponse

	for rows.Next() {
		var (
			id          uint64
			created     sql.NullTime
			title       sql.NullString
			description sql.NullString
		)

		if err := rows.Scan(&id, &created, &title, &description); err != nil {
			// TODO: map sql error to more fitting twirp error
			return nil, twirp.InternalErrorWith(err)
		}

		// it's not an error if any of these are empty
		task := pb.Task{
			Id:          id,
			Created:     timestamppb.New(created.Time),
			Title:       title.String,
			Description: description.String,
		}

		resp.Tasks = append(resp.Tasks, &task)
	}

	return &resp, nil
}

func (s *Server) CreateTask(ctx context.Context, req *pb.CreateTaskRequest) (*pb.CreateTaskResponse, error) {
	created := time.Now()

	res, err := s.stmtCache.ExecContext(
		ctx,
		"insert into tasks (created, title, description) values (?, ?, ?)",
		created, req.Title, req.Description)
	if err != nil {
		// TODO: map sql error to more fitting twirp error
		return nil, twirp.InternalErrorWith(err)
	}

	// should never get an error. record was inserted, so returning an error to
	// caller would be misleading
	id, _ := res.LastInsertId()

	task := pb.Task{
		Id:          uint64(id),
		Created:     timestamppb.New(created),
		Title:       req.Title,
		Description: req.Description,
	}

	resp := pb.CreateTaskResponse{
		Task: &task,
	}

	return &resp, err
}

func (s *Server) GetTask(ctx context.Context, req *pb.GetTaskRequest) (*pb.GetTaskResponse, error) {
	rows, err := s.stmtCache.QueryContext(ctx,
		"select id, created, title, description from tasks where id = ?",
		req.Id)
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	defer rows.Close()

	if !rows.Next() {
		return nil, twirp.NotFound.Errorf("task %d not found", req.Id)
	}
	var (
		id          uint64
		created     sql.NullTime
		title       sql.NullString
		description sql.NullString
	)

	if err := rows.Scan(&id, &created, &title, &description); err != nil {
		// TODO: map sql error to more fitting twirp error
		return nil, twirp.InternalErrorWith(err)
	}

	task := pb.Task{
		Id:          id,
		Created:     timestamppb.New(created.Time),
		Title:       title.String,
		Description: description.String,
	}

	resp := pb.GetTaskResponse{
		Task: &task,
	}

	return &resp, nil
}
