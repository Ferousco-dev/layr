package figmaurl

import (
	"errors"
	"strings"
	"testing"
)

const key = "AbCdEfGhIjKlMnOpQrStUv"

func TestParse(t *testing.T) {
	cases := []struct {
		name, in string
		want     Parsed
		invalid  bool
	}{
		{"design url", "https://www.figma.com/design/" + key + "/Landing-Page", Parsed{FileKey: key}, false},
		{"file url", "https://www.figma.com/file/" + key + "/Landing", Parsed{FileKey: key}, false},
		{"proto url", "https://www.figma.com/proto/" + key + "/Landing", Parsed{FileKey: key}, false},
		{"bare host", "https://figma.com/design/" + key, Parsed{FileKey: key}, false},
		{"trailing slash", "https://www.figma.com/design/" + key + "/", Parsed{FileKey: key}, false},
		{"upper case host", "https://WWW.FIGMA.COM/design/" + key, Parsed{FileKey: key}, false},
		{"explicit 443", "https://www.figma.com:443/design/" + key, Parsed{FileKey: key}, false},
		{"whitespace around", "  https://www.figma.com/design/" + key + "  ", Parsed{FileKey: key}, false},
		{"node id hyphen form", "https://www.figma.com/design/" + key + "/x?node-id=120-450", Parsed{key, "120:450"}, false},
		{"node id colon form", "https://www.figma.com/design/" + key + "/x?node-id=120:450", Parsed{key, "120:450"}, false},
		{"node id encoded colon", "https://www.figma.com/design/" + key + "/x?node-id=120%3A450", Parsed{key, "120:450"}, false},
		{"instance node id", "https://www.figma.com/design/" + key + "/x?node-id=I5-1%3B2-3", Parsed{key, "I5:1;2:3"}, false},
		{"extra query params", "https://www.figma.com/design/" + key + "/x?node-id=1-2&t=abc123&m=dev&p=f", Parsed{key, "1:2"}, false},
		{"fragment ignored", "https://www.figma.com/design/" + key + "/x?node-id=1-2#frag", Parsed{key, "1:2"}, false},
		{"empty node id", "https://www.figma.com/design/" + key + "/x?node-id=", Parsed{FileKey: key}, false},
		{"branch url uses branch key", "https://www.figma.com/design/" + key + "/branch/BranchKey123456/Name", Parsed{FileKey: "BranchKey123456"}, false},
		{"encoded file name", "https://www.figma.com/design/" + key + "/Caf%C3%A9%20%2F%20Menu", Parsed{FileKey: key}, false},

		{"empty", "", Parsed{}, true},
		{"spaces only", "   ", Parsed{}, true},
		{"not a url", "not a url", Parsed{}, true},
		{"missing file key", "https://www.figma.com/design/", Parsed{}, true},
		{"missing kind", "https://www.figma.com/" + key, Parsed{}, true},
		{"board unsupported", "https://www.figma.com/board/" + key + "/x", Parsed{}, true},
		{"make unsupported", "https://www.figma.com/make/" + key + "/x", Parsed{}, true},
		{"community page", "https://www.figma.com/community/file/123", Parsed{}, true},
		{"http scheme", "http://www.figma.com/design/" + key, Parsed{}, true},
		{"file scheme", "file:///etc/passwd", Parsed{}, true},
		{"javascript scheme", "javascript:alert(1)", Parsed{}, true},
		{"data scheme", "data:text/html,hi", Parsed{}, true},
		{"foreign host", "https://evil.example/design/" + key, Parsed{}, true},
		{"figma as subdomain of evil", "https://figma.com.evil.example/design/" + key, Parsed{}, true},
		{"evil as subdomain prefix", "https://evilfigma.com/design/" + key, Parsed{}, true},
		{"other figma subdomain", "https://api.figma.com/design/" + key, Parsed{}, true},
		{"userinfo trick", "https://www.figma.com@evil.example/design/" + key, Parsed{}, true},
		{"userinfo with figma host", "https://user:pw@www.figma.com/design/" + key, Parsed{}, true},
		{"localhost", "https://localhost/design/" + key, Parsed{}, true},
		{"ip host", "https://127.0.0.1/design/" + key, Parsed{}, true},
		{"odd port", "https://www.figma.com:8443/design/" + key, Parsed{}, true},
		{"key too short", "https://www.figma.com/design/abc", Parsed{}, true},
		{"key with slash encoded", "https://www.figma.com/design/ab%2Fcdefghij", Parsed{}, true},
		{"key with dots", "https://www.figma.com/design/..%2F..%2Fetc", Parsed{}, true},
		{"path traversal key", "https://www.figma.com/design/../../etc/passwd", Parsed{}, true},
		{"node id garbage", "https://www.figma.com/design/" + key + "/x?node-id=abc", Parsed{}, true},
		{"node id injection", "https://www.figma.com/design/" + key + "/x?node-id=1-2,3-4", Parsed{}, true},
		{"node id sql", "https://www.figma.com/design/" + key + "/x?node-id=1-2%27%3B--", Parsed{}, true},
		{"node id repeated", "https://www.figma.com/design/" + key + "/x?node-id=1-2&node-id=3-4", Parsed{}, true},
		{"node id only one part", "https://www.figma.com/design/" + key + "/x?node-id=12", Parsed{}, true},
		{"too long", "https://www.figma.com/design/" + key + "/" + strings.Repeat("a", 3000), Parsed{}, true},
		{"cyrillic a in host", "https://www.figm\u0430.com/design/" + key, Parsed{}, true},
		{"fullwidth host", "https://\uff46\uff49\uff47\uff4d\uff41.com/design/" + key, Parsed{}, true},
		{"punycode lookalike", "https://xn--figm-9cd.com/design/" + key, Parsed{}, true},
		{"trailing dot host", "https://www.figma.com./design/" + key, Parsed{}, true},
		{"encoded dot in host", "https://www%2efigma.com/design/" + key, Parsed{}, true},
		{"backslash authority", "https://evil.example\\@www.figma.com/design/" + key, Parsed{}, true},
		{"null byte", "https://www.figma.com/design/" + key + "\x00", Parsed{}, true},
	}
	for _, tc := range cases {
		got, err := Parse(tc.in)
		if tc.invalid {
			if !errors.Is(err, ErrInvalid) || got != (Parsed{}) {
				t.Errorf("%s: got %#v err %v, want ErrInvalid", tc.name, got, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%s: got %#v err %v, want %#v", tc.name, got, err, tc.want)
		}
	}
}

func FuzzParseNeverPanics(f *testing.F) {
	for _, seed := range []string{"https://www.figma.com/design/" + key + "/x?node-id=1-2", "", "%%%", "https://[::1", "https://figma.com/design//"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got, err := Parse(in)
		if err == nil && (got.FileKey == "" || strings.ContainsAny(got.FileKey, "/?# ")) {
			t.Fatalf("accepted unsafe key from %q: %#v", in, got)
		}
	})
}
