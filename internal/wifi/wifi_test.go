package wifi

import "testing"

func TestQRTextEscapes(t *testing.T) {
	got := QRText(`My;Net`, `p:a,s"s\`)
	want := `WIFI:T:WPA;S:My\;Net;P:p\:a\,s\"s\\;;`
	if got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
}
