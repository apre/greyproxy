package greywallapi

import "testing"

func TestMatchesContainer(t *testing.T) {
	tests := []struct {
		name      string
		container string
		pattern   string
		want      bool
	}{
		{"wildcard matches anything", "myapp", "*", true},
		{"exact match", "myapp", "myapp", true},
		{"exact no match", "myapp", "other", false},
		{"glob prefix", "myapp-worker", "myapp-*", true},
		{"glob prefix no match", "otherapp", "myapp-*", false},
		{"glob suffix", "worker-myapp", "*-myapp", true},
		{"question mark", "myapp1", "myapp?", true},
		{"question mark no match", "myapp12", "myapp?", false},
		{"empty container", "", "*", true},
		{"empty pattern", "myapp", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesContainer(tt.container, tt.pattern)
			if got != tt.want {
				t.Errorf("MatchesContainer(%q, %q) = %v, want %v", tt.container, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestMatchesDestination(t *testing.T) {
	tests := []struct {
		name string
		dest string
		pat  string
		want bool
	}{
		// Wildcard
		{"wildcard", "example.com", "*", true},

		// Exact match
		{"exact match", "example.com", "example.com", true},
		{"exact case insensitive", "Example.COM", "example.com", true},
		{"exact no match", "other.com", "example.com", false},

		// CIDR
		{"CIDR match", "192.168.1.100", "192.168.1.0/24", true},
		{"CIDR no match", "10.0.0.1", "192.168.1.0/24", false},
		{"CIDR hostname ignored", "example.com", "192.168.1.0/24", false},
		{"CIDR wide", "172.16.5.3", "172.16.0.0/12", true},

		// Double wildcard **.domain
		{"double wildcard root", "example.com", "**.example.com", true},
		{"double wildcard sub", "foo.example.com", "**.example.com", true},
		{"double wildcard deep sub", "a.b.example.com", "**.example.com", true},
		{"double wildcard no match", "other.com", "**.example.com", false},

		// Single wildcard *.domain
		{"single wildcard sub", "foo.example.com", "*.example.com", true},
		{"single wildcard deep sub", "a.b.example.com", "*.example.com", true},
		{"single wildcard root no match", "example.com", "*.example.com", false},
		{"single wildcard no match", "other.com", "*.example.com", false},

		// IP exact
		{"IP exact", "1.2.3.4", "1.2.3.4", true},
		{"IP no match", "1.2.3.5", "1.2.3.4", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesDestination(tt.dest, tt.pat)
			if got != tt.want {
				t.Errorf("MatchesDestination(%q, %q) = %v, want %v", tt.dest, tt.pat, got, tt.want)
			}
		})
	}
}

func TestMatchesPort(t *testing.T) {
	tests := []struct {
		name string
		port int
		pat  string
		want bool
	}{
		{"wildcard", 443, "*", true},
		{"exact match", 443, "443", true},
		{"exact no match", 80, "443", false},
		{"comma list match first", 80, "80,443", true},
		{"comma list match second", 443, "80,443", true},
		{"comma list no match", 8080, "80,443", false},
		{"range match low", 8000, "8000-9000", true},
		{"range match mid", 8500, "8000-9000", true},
		{"range match high", 9000, "8000-9000", true},
		{"range no match below", 7999, "8000-9000", false},
		{"range no match above", 9001, "8000-9000", false},
		{"comma with spaces", 443, "80, 443, 8080", true},
		{"invalid pattern", 80, "abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesPort(tt.port, tt.pat)
			if got != tt.want {
				t.Errorf("MatchesPort(%d, %q) = %v, want %v", tt.port, tt.pat, got, tt.want)
			}
		})
	}
}

func TestMatchesRule(t *testing.T) {
	tests := []struct {
		name      string
		container string
		dest      string
		port      int
		contPat   string
		destPat   string
		portPat   string
		want      bool
	}{
		{"all match", "myapp", "example.com", 443, "*", "**.example.com", "*", true},
		{"container no match", "other", "example.com", 443, "myapp", "**.example.com", "*", false},
		{"dest no match", "myapp", "other.com", 443, "*", "example.com", "*", false},
		{"port no match", "myapp", "example.com", 80, "*", "example.com", "443", false},
		{"all exact match", "myapp", "example.com", 443, "myapp", "example.com", "443", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesRule(tt.container, tt.dest, tt.port, tt.contPat, tt.destPat, tt.portPat)
			if got != tt.want {
				t.Errorf("MatchesRule() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculateSpecificity(t *testing.T) {
	tests := []struct {
		name    string
		contPat string
		destPat string
		portPat string
		want    int
	}{
		{"all wildcards", "*", "*", "*", 0},
		{"exact container only", "myapp", "*", "*", 200},
		{"wildcard container", "myapp-*", "*", "*", 100},
		{"exact dest only", "*", "example.com", "*", 20},
		{"wildcard dest", "*", "*.example.com", "*", 10},
		{"specific port", "*", "*", "443", 1},
		{"exact container + exact dest", "myapp", "example.com", "*", 220},
		{"all exact", "myapp", "example.com", "443", 221},
		{"wildcard container + wildcard dest + port", "myapp-*", "*.example.com", "443", 111},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateSpecificity(tt.contPat, tt.destPat, tt.portPat)
			if got != tt.want {
				t.Errorf("CalculateSpecificity(%q, %q, %q) = %d, want %d",
					tt.contPat, tt.destPat, tt.portPat, got, tt.want)
			}
		})
	}
}
