package console

import "testing"

func TestParamsStoresRunParameters(t *testing.T) {
	depthThree := 3
	tests := []struct {
		name        string
		path        string
		depth       *int
		replace     bool
		wantDepth   int
		wantDepthOK bool
	}{
		{
			name: "zero value",
		},
		{
			name: "unlimited depth",
			path: "/source",
		},
		{
			name:        "current directory only",
			path:        "/source",
			depth:       new(int),
			wantDepthOK: true,
		},
		{
			name:        "limited depth with replacement",
			path:        "/source/document.txt",
			depth:       &depthThree,
			replace:     true,
			wantDepth:   3,
			wantDepthOK: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := Params{
				Path:    test.path,
				Depth:   test.depth,
				Replace: test.replace,
			}

			if params.Path != test.path {
				t.Errorf("Path = %q, требуется %q", params.Path, test.path)
			}
			if params.Replace != test.replace {
				t.Errorf("Replace = %t, требуется %t", params.Replace, test.replace)
			}
			if (params.Depth != nil) != test.wantDepthOK {
				t.Fatalf("наличие Depth = %t, требуется %t", params.Depth != nil, test.wantDepthOK)
			}
			if params.Depth != nil && *params.Depth != test.wantDepth {
				t.Errorf("Depth = %d, требуется %d", *params.Depth, test.wantDepth)
			}
		})
	}
}
