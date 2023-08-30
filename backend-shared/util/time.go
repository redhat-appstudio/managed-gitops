package util

import "time"

// Clock interface is used to mock the functions provided by the time package.
type Clock interface {
	Now() time.Time
}

type clock struct{}

func (d *clock) Now() time.Time {
	return time.Now()
}

func NewClock() Clock {
	return &clock{}
}

// MockClock implements the Clock interface with a custom current time.
type MockClock struct {
	now time.Time
}

func NewMockClock(cur time.Time) *MockClock {
	return &MockClock{
		now: cur,
	}
}

func (m *MockClock) Now() time.Time {
	return m.now
}
