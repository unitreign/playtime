package gamelist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var pt = Entry{
	Path:      "./PlayTime/playtime.sh",
	Name:      "PlayTime",
	Desc:      "Local gameplay time tracker. See how long you've played every game.",
	Image:     "./PlayTime/icons/playtime.png",
	Thumbnail: "./PlayTime/icons/playtime.png",
}

const songo = `	<game id="7">
		<path>./songo5/Songo5.sh</path>
		<name>Songo #5</name>
		<image>./songo5/cover.png</image>
		<playcount>3</playcount>
		<lastplayed>20260915T124824</lastplayed>
		<gametime>359</gametime>
		<scrap name="ScreenScraper" date="20260905T191241" />
	</game>
`

const ptWithStats = `	<game>
		<path>./PlayTime/playtime.sh</path>
		<name>PlayTime</name>
		<desc>Local gameplay time tracker. See how long you've played every game.</desc>
		<image>./PlayTime/icons/playtime.png</image>
		<thumbnail>./PlayTime/icons/playtime.png</thumbnail>
		<playcount>45</playcount>
		<lastplayed>20260915T211504</lastplayed>
		<gametime>3550</gametime>
	</game>
`

func doc(games ...string) string {
	return "<?xml version=\"1.0\"?>\n<gameList>\n" + strings.Join(games, "") + "</gameList>\n"
}

func TestUpToDateIsUntouched(t *testing.T) {
	in := doc(songo, ptWithStats)
	out, changed, err := upsert(in, pt)
	if err != nil || changed || out != in {
		t.Fatalf("expected no change, got changed=%v err=%v\n%s", changed, err, out)
	}
}

func TestAddKeepsOtherEntries(t *testing.T) {
	in := doc(songo)
	out, changed, err := upsert(in, pt)
	if err != nil || !changed {
		t.Fatalf("expected change, err=%v", err)
	}
	if !strings.Contains(out, songo) {
		t.Fatalf("other entry was modified:\n%s", out)
	}
	if !strings.Contains(out, "<path>./PlayTime/playtime.sh</path>") {
		t.Fatalf("entry not added:\n%s", out)
	}
	// Adding again must be a no-op.
	if _, again, _ := upsert(out, pt); again {
		t.Fatal("second upsert should not change anything")
	}
}

func TestUpdateKeepsStats(t *testing.T) {
	old := strings.Replace(ptWithStats, "./PlayTime/icons/playtime.png</image>", "./PlayTime/old.png</image>", 1)
	old = strings.Replace(old, "\t\t<desc>Local gameplay time tracker. See how long you've played every game.</desc>\n", "", 1)
	in := doc(songo, old)
	out, changed, err := upsert(in, pt)
	if err != nil || !changed {
		t.Fatalf("expected change, err=%v", err)
	}
	for _, want := range []string{
		songo,
		"<image>./PlayTime/icons/playtime.png</image>",
		"<desc>Local gameplay time tracker. See how long you've played every game.</desc>",
		"<playcount>45</playcount>", "<lastplayed>20260915T211504</lastplayed>", "<gametime>3550</gametime>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "old.png") {
		t.Fatalf("old image not replaced:\n%s", out)
	}
	if _, again, _ := upsert(out, pt); again {
		t.Fatal("second upsert should not change anything")
	}
}

func TestSelfClosingField(t *testing.T) {
	in := doc(strings.Replace(ptWithStats, "<desc>Local gameplay time tracker. See how long you've played every game.</desc>", "<desc />", 1))
	out, _, err := upsert(in, pt)
	if err != nil || out != doc(ptWithStats) {
		t.Fatalf("unexpected result (err=%v):\n%s", err, out)
	}
}

func TestEscapedValuesCompareEqual(t *testing.T) {
	in := doc(strings.Replace(ptWithStats, "you've", "you&apos;ve", 1))
	if _, changed, _ := upsert(in, pt); changed {
		t.Fatal("entity-escaped desc should count as equal")
	}
}

func TestBrokenFileRefused(t *testing.T) {
	if _, _, err := upsert("<?xml version=\"1.0\"?>\n<gameList>\n\t<game><path>x", pt); err == nil {
		t.Fatal("expected error for truncated file")
	}
}

func TestRegisterFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gamelist.xml")

	// Missing file gets created.
	if err := Register(p, pt); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "<name>PlayTime</name>") {
		t.Fatalf("not created:\n%s", got)
	}

	// Up-to-date file is not rewritten.
	os.WriteFile(p, []byte(doc(songo, ptWithStats)), 0644)
	before, _ := os.Stat(p)
	if err := Register(p, pt); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(p)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("file was rewritten even though nothing changed")
	}
	if _, err := os.Stat(p + ".playtime.tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file left behind")
	}
}
