package lowlevel_test

import (
	"github.com/stretchr/testify/mock"
	"os"
)

type MockExecutor struct {
	mock.Mock
}

func (me *MockExecutor) Run(prog string, args []string) error {
	c := me.Called(prog, args)
	return c.Error(0)
}

func (e *MockExecutor) RunExitCode(prog string, args []string) (int, error) {
	c := e.Called(prog, args)
	return c.Int(0), c.Error(1)
}

func (me *MockExecutor) Read(prog string, args []string) (string, error) {
	c := me.Called(prog, args)
	return c.String(0), c.Error(1)
}

func (me *MockExecutor) ReadFile(fn string) ([]byte, error) {
	c := me.Called(fn)
	return c.Get(0).([]byte), c.Error(1)
}

func (me *MockExecutor) WriteFile(fn string, content []byte, perm os.FileMode) error {
	c := me.Called(fn, content, perm)
	return c.Error(0)
}
