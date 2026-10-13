package notify

import (
	"testing"
	"time"
)

func TestForConfirmedPlansOnlyFutureReminders(t *testing.T) {
	now := time.Date(2026, time.October, 12, 8, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		start time.Time
		want  []Planned
	}{
		{
			name:  "three days ahead",
			start: now.Add(72 * time.Hour),
			want: []Planned{
				{KindConfirmation, now},
				{KindReminder24h, now.Add(48 * time.Hour)},
				{KindReminder2h, now.Add(70 * time.Hour)},
			},
		},
		{
			name:  "five hours ahead",
			start: now.Add(5 * time.Hour),
			want:  []Planned{{KindConfirmation, now}, {KindReminder2h, now.Add(3 * time.Hour)}},
		},
		{
			name:  "exactly two hours ahead",
			start: now.Add(2 * time.Hour),
			want:  []Planned{{KindConfirmation, now}},
		},
		{
			name:  "ninety minutes ahead",
			start: now.Add(90 * time.Minute),
			want:  []Planned{{KindConfirmation, now}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ForConfirmed(c.start, now)
			if len(got) != len(c.want) {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
			for i := range got {
				if got[i].Kind != c.want[i].Kind || !got[i].SendAt.Equal(c.want[i].SendAt) {
					t.Fatalf("got %+v, want %+v", got, c.want)
				}
			}
		})
	}
}

func TestBackoffDoublesUpToACap(t *testing.T) {
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute}
	for i, w := range want {
		if got := Backoff(i + 1); got != w {
			t.Errorf("Backoff(%d) = %v, want %v", i+1, got, w)
		}
	}
	if got := Backoff(30); got != time.Hour {
		t.Errorf("Backoff(30) = %v, want the 1h cap", got)
	}
	if got := Backoff(0); got != 30*time.Second {
		t.Errorf("Backoff(0) = %v", got)
	}
}
