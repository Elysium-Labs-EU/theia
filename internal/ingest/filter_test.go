package ingest

import "testing"

func TestAllowPageView(t *testing.T) {
	tests := []struct {
		name string
		f    Filter
		pv   PageView
		want bool
	}{
		{
			name: "no filter allows everything",
			f:    Filter{},
			pv:   PageView{Host: "example.com", Path: "/x"},
			want: true,
		},
		{
			name: "exclude host blocks matching host",
			f:    Filter{ExcludeHosts: []string{"navidrome.home.rtgs.me"}},
			pv:   PageView{Host: "navidrome.home.rtgs.me", Path: "/rest/ping"},
			want: false,
		},
		{
			// allowPageView itself does a plain string match — normalization
			// to lowercase is NewFilter's job (see TestNewFilter), and pv.Host
			// is already normalized by parseNginxLog by the time it gets here.
			name: "exclude host via NewFilter is case-insensitive",
			f:    NewFilter(nil, []string{"Navidrome.Home.RTGS.me"}, nil),
			pv:   PageView{Host: "navidrome.home.rtgs.me"},
			want: false,
		},
		{
			name: "exclude host lets other hosts through",
			f:    Filter{ExcludeHosts: []string{"navidrome.home.rtgs.me"}},
			pv:   PageView{Host: "elysiumlabs.dev"},
			want: true,
		},
		{
			name: "include host allows only listed hosts",
			f:    Filter{IncludeHosts: []string{"elysiumlabs.dev"}},
			pv:   PageView{Host: "elysiumlabs.dev"},
			want: true,
		},
		{
			name: "include host blocks unlisted hosts",
			f:    Filter{IncludeHosts: []string{"elysiumlabs.dev"}},
			pv:   PageView{Host: "navidrome.home.rtgs.me"},
			want: false,
		},
		{
			name: "exclude path blocks matching prefix",
			f:    Filter{ExcludePaths: []string{"/rest/ping"}},
			pv:   PageView{Path: "/rest/ping?u=x&t=y"},
			want: false,
		},
		{
			name: "exclude path lets other paths through",
			f:    Filter{ExcludePaths: []string{"/rest/ping"}},
			pv:   PageView{Path: "/"},
			want: true,
		},
		{
			name: "exclude host wins even if include host would allow it",
			f:    Filter{IncludeHosts: []string{"navidrome.home.rtgs.me"}, ExcludeHosts: []string{"navidrome.home.rtgs.me"}},
			pv:   PageView{Host: "navidrome.home.rtgs.me"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allowPageView(tt.f, &tt.pv); got != tt.want {
				t.Errorf("allowPageView(%+v, %+v) = %v, want %v", tt.f, tt.pv, got, tt.want)
			}
		})
	}
}

func TestNewFilter(t *testing.T) {
	f := NewFilter([]string{"Example.COM"}, []string{"Other.COM"}, []string{"/rest/ping"})

	if f.IncludeHosts[0] != "example.com" {
		t.Errorf("IncludeHosts[0] = %q, want normalized lowercase", f.IncludeHosts[0])
	}
	if f.ExcludeHosts[0] != "other.com" {
		t.Errorf("ExcludeHosts[0] = %q, want normalized lowercase", f.ExcludeHosts[0])
	}
	if f.ExcludePaths[0] != "/rest/ping" {
		t.Errorf("ExcludePaths[0] = %q, want unchanged", f.ExcludePaths[0])
	}
}
