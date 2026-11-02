package translator_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/JoaoVitorOli/althea/extensions/translator"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

// TestMain doubles as the engine: a test starts this binary with fakeEngine set.
func TestMain(m *testing.M) {
	if os.Getenv(fakeEngine) != "" {
		runFakeEngine()
		return
	}
	goleak.VerifyTestMain(m)
}

const fakeEngine = "ALTHEA_FAKE_BERGAMOT"

// runFakeEngine speaks the protocol of tools/bergamot/main.cpp: it marks text with the pair, needs
// the pair's config in its working directory, and counts the requests it heard.
func runFakeEngine() {
	in, out := bufio.NewReader(os.Stdin), bufio.NewWriter(os.Stdout)
	for {
		var pair string
		var size int
		if _, err := fmt.Fscanf(in, "%s %d\n", &pair, &size); err != nil {
			return
		}
		text := make([]byte, size)
		if _, err := io.ReadFull(in, text); err != nil {
			return
		}
		answer, status := "["+pair+"] "+string(text), "ok"
		if _, err := os.Stat(filepath.Join(pair, "config.yml")); err != nil {
			answer, status = "no model", "error"
		}
		switch string(text) {
		case "hang":
			time.Sleep(time.Hour)
		case "die":
			os.Exit(3)
		}
		fmt.Fprintf(out, "%s %d\n%s", status, len(answer), answer)
		_ = out.Flush()
	}
}

// files serves downloads by path, counting requests.
type files struct {
	srv      *httptest.Server
	contents map[string][]byte
	hits     map[string]int
}

func serveFiles(t *testing.T) *files {
	t.Helper()
	f := &files{contents: map[string][]byte{}, hits: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits[r.URL.Path]++
		data, ok := f.contents[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// add serves content and returns its pin.
func (f *files) add(role, path, content string) translator.File {
	f.contents[path] = []byte(content)
	sum := sha256.Sum256([]byte(content))
	return translator.File{Role: role, URL: f.srv.URL + path, Size: int64(len(content)), SHA256: hex.EncodeToString(sum[:])}
}

func (f *files) pair(from, to string) translator.Pair {
	key := from + "-" + to
	return translator.Pair{From: from, To: to, Files: []translator.File{
		f.add("lex", "/"+key+"/lex", "lex of "+key),
		f.add("model", "/"+key+"/model", "model of "+key),
		f.add("vocab", "/"+key+"/vocab", "vocab of "+key),
	}}
}

// engineExe is a copy of this binary to serve as the engine's download.
func engineContent(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

type fixture struct {
	*sdktest.Context
	e     *translator.Extension
	files *files
	dir   string
}

// setUp makes an extension whose models and engine come from a local server. It has en↔pt, en→es and
// fr→en.
func setUp(t *testing.T, lang string) *fixture {
	t.Helper()
	f := serveFiles(t)
	dir := t.TempDir()
	engine := f.add("", "/engine", engineContent(t))
	e := translator.New(translator.Platform{
		Dir: dir,
		Command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Env = append(os.Environ(), fakeEngine+"=1")
			return cmd
		},
		Models: []translator.Pair{f.pair("en", "pt"), f.pair("pt", "en"), f.pair("en", "es"), f.pair("fr", "en")},
		Engine: &engine,
	})
	ctx := sdktest.New(t, e, lang)
	stop := serve(t, e, ctx)
	t.Cleanup(stop)
	return &fixture{ctx, e, f, dir}
}

// serve runs the service as the host does, and stops it as the test ends, which ends the engine.
func serve(t *testing.T, e *translator.Extension, sc *sdktest.Context) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s := *contextWith(ctx, sc)
	go func() {
		defer close(done)
		_ = e.Serve(s)
	}()
	return func() {
		cancel()
		<-done
	}
}

// withContext is a Context with another cancellation.
type withContext struct {
	*sdktest.Context
	ctx context.Context
}

func (w withContext) Done() <-chan struct{} { return w.ctx.Done() }
func (w withContext) Err() error            { return w.ctx.Err() }

func contextWith(ctx context.Context, sc *sdktest.Context) *withContext {
	return &withContext{sc, ctx}
}

func (f *fixture) command(t *testing.T, id string) sdk.Command {
	t.Helper()
	for _, c := range f.e.Commands(f.Context) {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no command %q", id)
	return sdk.Command{}
}

// stream runs a Detail's Stream and returns what shows in the end.
func stream(ctx sdk.Context, d *sdk.Detail) (string, error) {
	var shown string
	err := d.Stream(ctx, func(s string) { shown = s })
	return shown, err
}

func lastPushed[T sdk.View](t *testing.T, f *fixture) T {
	t.Helper()
	v, ok := f.Pushed[len(f.Pushed)-1].(T)
	if !ok {
		t.Fatalf("pushed %T", f.Pushed[len(f.Pushed)-1])
	}
	return v
}

func TestThePinnedModelsAreComplete(t *testing.T) {
	e := translator.New(translator.Platform{})
	ctx := sdktest.New(t, e, "en")
	var languages []string
	for _, p := range e.Preferences(ctx) {
		if p.Name == "target" {
			for _, o := range p.Options[1:] {
				languages = append(languages, o.Value)
				if strings.Contains(o.Title, "lang.") || o.Title == "" {
					t.Errorf("%s has no name: %q", o.Value, o.Title)
				}
			}
		}
	}
	if len(languages) < 30 || !slices.Contains(languages, "pt") || !slices.Contains(languages, "en") {
		t.Errorf("languages %v", languages)
	}
	pt := sdktest.New(t, e, "pt-BR")
	for _, lang := range languages {
		if got := pt.T("lang." + lang); got == "" || strings.Contains(got, "lang.") {
			t.Errorf("%s has no name in Portuguese: %q", lang, got)
		}
	}
}

func TestPinnedFilesHaveHashesAndSizes(t *testing.T) {
	data, err := os.ReadFile("models.json")
	if err != nil {
		t.Fatal(err)
	}
	var pairs []translator.Pair
	if err := json.Unmarshal(data, &pairs); err != nil {
		t.Fatal(err)
	}
	if len(pairs) == 0 {
		t.Fatal("no models")
	}
	for _, p := range pairs {
		if len(p.Files) != 3 {
			t.Errorf("%s has %d files", p.Key(), len(p.Files))
		}
		for _, f := range p.Files {
			if len(f.SHA256) != 64 || f.Size <= 0 || !strings.HasPrefix(f.URL, "https://") {
				t.Errorf("%s %s: %+v", p.Key(), f.Role, f)
			}
		}
	}
	var engines map[string]translator.File
	if err := readJSON("engine.json", &engines); err != nil {
		t.Fatal(err)
	}
	if _, ok := engines["windows/amd64"]; !ok {
		t.Errorf("engines %v", engines)
	}
}

func readJSON(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func TestNothingLeavesNorDownloadsAsItStarts(t *testing.T) {
	f := setUp(t, "en")
	_ = f.e.Commands(f.Context)
	_ = f.e.Fallbacks(f.Context, "bom dia")
	_ = f.e.Preferences(f.Context)
	if len(f.Requests) != 0 || f.Selections != 0 {
		t.Errorf("requests %v, selections %d", f.Requests, f.Selections)
	}
}

func TestOfflineAsksBeforeDownloadingThenTranslatesAndCaches(t *testing.T) {
	f := setUp(t, "pt-BR")
	f.Selection = "hello there my friend"
	if err := f.command(t, "translate").Run(f); err != nil {
		t.Fatal(err)
	}
	if len(f.Requests) != 0 {
		t.Fatalf("downloaded before asking: %v", f.Requests)
	}
	offer := lastPushed[*sdk.Detail](t, f)
	if offer.Stream != nil || !strings.Contains(offer.Markdown, "Inglês para Português") || !strings.Contains(offer.Markdown, "motor") {
		t.Errorf("offer %q", offer.Markdown)
	}

	if err := offer.Actions[0].Run(f); err != nil {
		t.Fatal(err)
	}
	result := lastPushed[*sdk.Detail](t, f)
	if f.Popped != 1 || result.Title != "Inglês → Português" {
		t.Errorf("popped %d, title %q", f.Popped, result.Title)
	}
	var shown []string
	err := result.Stream(f, func(s string) { shown = append(shown, s) })
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(shown, "Baixando… 100%") {
		t.Errorf("no progress shown: %q", shown)
	}
	if got := shown[len(shown)-1]; got != `\[en-pt\] hello there my friend` {
		t.Errorf("shown %q", got)
	}
	if !strings.Contains(result.Metadata[0].Text, "detectado") {
		t.Errorf("metadata %+v", result.Metadata)
	}

	// Copy gives the translation without Markdown's escapes.
	if err := result.Actions[0].Run(f); err != nil {
		t.Fatal(err)
	}
	if got := f.Copied[len(f.Copied)-1]; got != "[en-pt] hello there my friend" {
		t.Errorf("copied %q", got)
	}

	// Again: no offer, no download, from the cache.
	requests := len(f.Requests)
	if err := f.command(t, "translate").Run(f); err != nil {
		t.Fatal(err)
	}
	again := lastPushed[*sdk.Detail](t, f)
	if again.Stream == nil {
		t.Fatal("offered again what is there")
	}
	if got, err := stream(f, again); err != nil || got != `\[en-pt\] hello there my friend` {
		t.Errorf("shown %q, err %v", got, err)
	}
	if len(f.Requests) != requests {
		t.Errorf("requests %v", f.Requests)
	}
}

// install downloads en↔pt and en→es through the views.
func (f *fixture) install(t *testing.T, text, from, to string) string {
	t.Helper()
	form := f.form(t)
	if err := form.Submit(f, sdk.Values{"text": text, "from": from, "to": to}); err != nil {
		t.Fatal(err)
	}
	d := lastPushed[*sdk.Detail](t, f)
	if d.Stream == nil {
		if err := d.Actions[0].Run(f); err != nil {
			t.Fatal(err)
		}
		d = lastPushed[*sdk.Detail](t, f)
	}
	got, err := stream(f, d)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (f *fixture) form(t *testing.T) *sdk.Form {
	t.Helper()
	f.Selection = ""
	if err := f.command(t, "translate").Run(f); err != nil {
		t.Fatal(err)
	}
	return lastPushed[*sdk.Form](t, f)
}

func TestWithoutASelectionItAsksWhatToTranslateAndRemembersTheLanguage(t *testing.T) {
	f := setUp(t, "en")
	form := f.form(t)
	if form.Fields[2].Default != "pt" && form.Fields[2].Default != "en" {
		t.Errorf("target %q", form.Fields[2].Default)
	}
	if got := f.install(t, "good morning", "en", "es"); got != `\[en-es\] good morning` {
		t.Errorf("shown %q", got)
	}
	if got := f.form(t).Fields[2].Default; got != "es" {
		t.Errorf("target %q, want the last one", got)
	}
}

func TestOtherPairsGoThroughEnglish(t *testing.T) {
	f := setUp(t, "en")
	got := f.install(t, "bom dia", "pt", "es")
	if got != `\[en-es\] \[pt-en\] bom dia` {
		t.Errorf("shown %q", got)
	}
}

func TestAPairWithNoModelSaysSoWithoutDownloading(t *testing.T) {
	f := setUp(t, "en")
	form := f.form(t)
	err := form.Submit(f, sdk.Values{"text": "olá", "from": "pt", "to": "fr"})
	if err == nil || !strings.Contains(err.Error(), "no offline model from Portuguese to French") {
		t.Errorf("err %v", err)
	}
	if len(f.Requests) != 0 {
		t.Errorf("requests %v", f.Requests)
	}
}

func TestADownloadThatDoesNotVerifyIsThrownAway(t *testing.T) {
	f := setUp(t, "en")
	f.files.contents["/en-pt/model"] = []byte("model of en-XX")
	form := f.form(t)
	if err := form.Submit(f, sdk.Values{"text": "hi", "from": "en", "to": "pt"}); err != nil {
		t.Fatal(err)
	}
	if err := lastPushed[*sdk.Detail](t, f).Actions[0].Run(f); err != nil {
		t.Fatal(err)
	}
	_, err := stream(f, lastPushed[*sdk.Detail](t, f))
	if err == nil || !strings.Contains(err.Error(), "did not match") {
		t.Fatalf("err %v", err)
	}
	var found []string
	_ = filepath.WalkDir(filepath.Join(f.dir, "models"), func(path string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() {
			found = append(found, filepath.Base(path))
		}
		return nil
	})
	if slices.Contains(found, "model.bin") || slices.ContainsFunc(found, func(n string) bool { return strings.HasPrefix(n, "download-") }) {
		t.Errorf("left behind: %v", found)
	}
}

func TestAnEngineChangedSinceTheDownloadDoesNotRun(t *testing.T) {
	f := setUp(t, "en")
	f.install(t, "first", "en", "pt")
	var engine string
	_ = filepath.WalkDir(filepath.Join(f.dir, "engine"), func(path string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() {
			engine = path
		}
		return nil
	})
	// Stop what runs, then change the file: same size, other content.
	f.e.Stop()
	data, err := os.ReadFile(engine)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 0xff
	if err := os.WriteFile(engine, data, 0o600); err != nil {
		t.Fatal(err)
	}
	form := f.form(t)
	if err := form.Submit(f, sdk.Values{"text": "second", "from": "en", "to": "pt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := stream(f, lastPushed[*sdk.Detail](t, f)); err == nil {
		t.Error("the changed engine ran")
	}
}

func TestStoppingATranslationStopsTheEngineAndTheNextStartsItAgain(t *testing.T) {
	f := setUp(t, "en")
	f.install(t, "warm up", "en", "pt")
	form := f.form(t)
	if err := form.Submit(f, sdk.Values{"text": "hang", "from": "en", "to": "pt"}); err != nil {
		t.Fatal(err)
	}
	d := lastPushed[*sdk.Detail](t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	s := contextWith(ctx, f.Context)
	if _, err := stream(s, d); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err %v", err)
	}
	if got := f.install(t, "after", "en", "pt"); got != `\[en-pt\] after` {
		t.Errorf("shown %q", got)
	}
}

func TestAnEngineThatDiesIsSaidPlainlyAndStartsAgain(t *testing.T) {
	f := setUp(t, "en")
	f.install(t, "warm up", "en", "pt")
	form := f.form(t)
	if err := form.Submit(f, sdk.Values{"text": "die", "from": "en", "to": "pt"}); err != nil {
		t.Fatal(err)
	}
	_, err := stream(f, lastPushed[*sdk.Detail](t, f))
	if err == nil || !strings.Contains(err.Error(), "offline translation failed") {
		t.Errorf("err %v", err)
	}
	if got := f.install(t, "after", "en", "pt"); got != `\[en-pt\] after` {
		t.Errorf("shown %q", got)
	}
}

func TestDeepLSendsWhatItShouldAndNothingWithoutAKey(t *testing.T) {
	f := setUp(t, "en")
	var heard map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &heard)
		auth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"translations":[{"text":"Bom dia"}]}`)
	}))
	defer srv.Close()
	f.e.DeepLBase = srv.URL
	f.Prefs["provider"] = "deepl"

	f.Selection = "good morning"
	if err := f.command(t, "translate").Run(f); err != nil {
		t.Fatal(err)
	}
	if _, err := stream(f, lastPushed[*sdk.Detail](t, f)); err == nil || !strings.Contains(err.Error(), "DeepL needs an API key") {
		t.Fatalf("err %v", err)
	}
	if len(f.Requests) != 0 {
		t.Fatalf("requests %v", f.Requests)
	}

	f.Secrets["deepl_key"] = "abc:fx"
	got, err := stream(f, lastPushed[*sdk.Detail](t, f))
	if err != nil || got != "Bom dia" {
		t.Fatalf("shown %q, err %v", got, err)
	}
	if auth != "DeepL-Auth-Key abc:fx" || heard["target_lang"] != "PT-BR" || heard["source_lang"] != "EN" {
		t.Errorf("auth %q, body %v", auth, heard)
	}
}

func TestLibreTranslateAsksForItsServer(t *testing.T) {
	f := setUp(t, "en")
	var heard map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &heard)
		if r.URL.Path != "/translate" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"translatedText":"Olá"}`)
	}))
	defer srv.Close()
	f.Prefs["provider"] = "libretranslate"
	f.Selection = "hello there"
	if err := f.command(t, "translate").Run(f); err != nil {
		t.Fatal(err)
	}
	d := lastPushed[*sdk.Detail](t, f)
	if _, err := stream(f, d); err == nil || !strings.Contains(err.Error(), "LibreTranslate server") {
		t.Fatalf("err %v", err)
	}
	f.Prefs["libretranslate_url"] = srv.URL + "/"
	f.Secrets["libretranslate_key"] = "k"
	got, err := stream(f, d)
	if err != nil || got != "Olá" {
		t.Fatalf("shown %q, err %v", got, err)
	}
	if heard["target"] != "pt" && heard["target"] != "en" || heard["api_key"] != "k" {
		t.Errorf("body %v", heard)
	}
}

func TestTheAIProviderTranslatesAsItWrites(t *testing.T) {
	f := setUp(t, "en")
	var system, asked string
	var calls int
	e := translator.New(translator.Platform{
		Dir: f.dir,
		AI: func(_ sdk.Context, sys, text string, show func(string)) (string, error) {
			calls++
			system, asked = sys, text
			show("Bom")
			show("Bom dia")
			return "Bom dia", nil
		},
	})
	ctx := sdktest.New(t, e, "pt-BR")
	ctx.Prefs["provider"] = "ai"
	ctx.Selection = "good morning"
	if err := e.Commands(ctx)[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	d := ctx.Pushed[0].(*sdk.Detail)
	var shown []string
	if err := d.Stream(ctx, func(s string) { shown = append(shown, s) }); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(shown, []string{"Bom", "Bom dia", "Bom dia"}) {
		t.Errorf("shown %q", shown)
	}
	if asked != "good morning" || !strings.Contains(system, `"en"`) || !strings.Contains(system, `"pt"`) {
		t.Errorf("asked %q, system %q", asked, system)
	}

	// The cache answers the same again, without asking.
	if err := d.Stream(ctx, func(string) {}); err != nil || calls != 1 {
		t.Errorf("calls %d, err %v", calls, err)
	}
}

func TestTranslatingBackSwapsTheLanguages(t *testing.T) {
	f := setUp(t, "en")
	f.install(t, "hello", "en", "pt")
	if err := lastPushed[*sdk.Detail](t, f).Actions[2].Run(f); err != nil {
		t.Fatal(err)
	}
	d := lastPushed[*sdk.Detail](t, f)
	if d.Stream == nil {
		if err := d.Actions[0].Run(f); err != nil {
			t.Fatal(err)
		}
		d = lastPushed[*sdk.Detail](t, f)
	}
	if got, err := stream(f, d); err != nil || got != `\[pt-en\] \[en-pt\] hello` {
		t.Errorf("shown %q, err %v", got, err)
	}
}

func TestTheLanguagesListOffersDownloadsAndRemovesThem(t *testing.T) {
	f := setUp(t, "en")
	if err := f.command(t, "models").Run(f); err != nil {
		t.Fatal(err)
	}
	items, err := lastPushed[*sdk.List](t, f).Load(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Requests) != 0 {
		t.Fatalf("requests %v", f.Requests)
	}
	i := slices.IndexFunc(items, func(it sdk.Item) bool { return it.ID == "pt" })
	if i < 0 || items[i].Section != "To download" || items[i].Actions[0].Confirm == "" {
		t.Fatalf("items %+v", items)
	}
	if err := items[i].Actions[0].Run(f); err != nil {
		t.Fatal(err)
	}
	if got, err := stream(f, lastPushed[*sdk.Detail](t, f)); err != nil || got != "Downloaded." {
		t.Fatalf("shown %q, err %v", got, err)
	}

	if err := f.command(t, "models").Run(f); err != nil {
		t.Fatal(err)
	}
	items, err = lastPushed[*sdk.List](t, f).Load(f)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].ID != "pt" || items[0].Section != "On this computer" {
		t.Fatalf("items %+v", items)
	}
	if err := items[0].Actions[0].Run(f); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "models", "en-pt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("still there: %v", err)
	}
}

func TestOfflineWithoutAnEngineInThisVersionSaysSo(t *testing.T) {
	dir := t.TempDir()
	e := translator.New(translator.Platform{Dir: dir, Engine: &translator.File{}})
	ctx := sdktest.New(t, e, "en")
	ctx.Selection = "hello there"
	err := e.Commands(ctx)[0].Run(ctx)
	if err == nil || !strings.Contains(err.Error(), "not available in this version") {
		t.Errorf("err %v", err)
	}
	if len(ctx.Pushed) != 0 || len(ctx.Requests) != 0 {
		t.Errorf("pushed %v, requests %v", ctx.Pushed, ctx.Requests)
	}
}

func TestDetect(t *testing.T) {
	for text, want := range map[string]string{
		"the quick fox is in the house and it was good":        "en",
		"o gato está em cima da mesa e não quer sair de lá":    "pt",
		"el gato está en la casa y no quiere salir del jardín": "es",
		"le chat est dans la maison et je ne sais pas pour ça": "fr",
		"der Hund ist nicht in dem Haus und ich bin müde":      "de",
		"Привет, как дела":                                     "ru",
		"Καλημέρα κόσμε":                                       "el",
		"שלום עולם":                                            "he",
		"123 456":                                              "",
	} {
		if got := translator.Detect(text); got != want {
			t.Errorf("Detect(%q) = %q, want %q", text, got, want)
		}
	}
}
