package resource

import "testing"

func TestGhTargetURL(t *testing.T) {
	testCases := []struct {
		name         string
		atc          string
		team         string
		pipeline     string
		job          string
		buildN       string
		instanceVars string
		want         string
	}{
		{
			name: "all defaults",
			want: "https://ci.example.com/teams/devs/pipelines/magritte/jobs/paint/builds/42",
		},
		{
			name:         "instanced vars 1",
			instanceVars: `{"branch":"stable"}`,
			want:         "https://ci.example.com/teams/devs/pipelines/magritte/jobs/paint/builds/42?vars=%7B%22branch%22%3A%22stable%22%7D",
		},
		{
			name:         "instanced vars 2",
			instanceVars: `{"branch":"stable","foo":"bar"}`,
			want:         "https://ci.example.com/teams/devs/pipelines/magritte/jobs/paint/builds/42?vars=%7B%22branch%22%3A%22stable%22%2C%22foo%22%3A%22bar%22%7D",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.want == "" {
				t.Fatal("tc.want: empty")
			}

			atc := "https://ci.example.com"
			if tc.atc != "" {
				atc = tc.atc
			}
			team := "devs"
			if tc.team != "" {
				team = tc.team
			}
			pipeline := "magritte"
			if tc.pipeline != "" {
				pipeline = tc.pipeline
			}
			job := "paint"
			if tc.job != "" {
				job = tc.job
			}
			buildN := "42"
			if tc.buildN != "" {
				buildN = tc.buildN
			}

			have := ghTargetURL(atc, team, pipeline, job, buildN, tc.instanceVars)

			if have != tc.want {
				t.Fatalf("\nhave: %s\nwant: %s", have, tc.want)
			}
		})
	}
}
