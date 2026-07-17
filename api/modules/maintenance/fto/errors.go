package fto

import "errors"

// ErrTaskNotFound is returned when an FTO update references a task_id that does not exist.
var ErrTaskNotFound = errors.New("task not found")
