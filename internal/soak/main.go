//go:build windows

// Command soak measures a build of Althea against the budgets of docs/spec.md, and runs it for
// hours to find what leaks. It runs a portable copy, with data of its own, so the user's are
// never touched; it never captures the screen.
//
//	go build -o althea.exe ./cmd/althea
//	go run ./internal/soak -exe althea.exe                    # cold start, idle memory and CPU
//	go run ./internal/soak -exe althea.exe -hotkey            # and hotkey to visible window
//	go run ./internal/soak -exe althea.exe -soak 24h -csv soak.csv
//
// -hotkey and -soak press Alt+Space and Escape on the desktop: run them while away from it.
package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/JoaoVitorOli/althea/internal/store"
)

func main() {
	exe := flag.String("exe", "", "the althea.exe to measure")
	count := flag.Int("count", 3, "how many cold starts to measure")
	idle := flag.Duration("idle", 15*time.Second, "how long Althea idles before its memory is read")
	hotkey := flag.Bool("hotkey", false, "measure hotkey to visible window, pressing Alt+Space and Escape")
	soak := flag.Duration("soak", 0, "run this long, showing and hiding the launcher, sampling resources")
	every := flag.Duration("every", time.Minute, "how often the soak samples")
	toggle := flag.Duration("toggle", 20*time.Second, "how often the soak shows and hides the launcher")
	csvPath := flag.String("csv", "soak.csv", "where the soak writes its samples")
	flag.Parse()
	if *exe == "" {
		flag.Usage()
		os.Exit(2)
	}
	var err error
	if *soak > 0 {
		err = runSoak(*exe, *soak, *every, *toggle, *csvPath)
	} else {
		err = runMeasure(*exe, *count, *idle, *hotkey)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "soak:", err)
		os.Exit(1)
	}
}

// instance is a portable copy of Althea, running.
type instance struct {
	dir     string
	cmd     *exec.Cmd
	started time.Time
	exited  chan error
}

// install copies exe to a new portable directory, once: the antivirus scans a new executable as
// it first runs, which no cold start after the first pays.
func install(exe string) (string, error) {
	dir, err := os.MkdirTemp("", "althea-soak-*")
	if err != nil {
		return "", err
	}
	if err := copyFile(exe, filepath.Join(dir, "althea.exe")); err != nil {
		return "", err
	}
	return dir, os.WriteFile(filepath.Join(dir, "althea.portable"), nil, 0o600)
}

// launch starts the copy in dir with data of its own, the onboarding done and start at login
// off, as if at login, so that the launcher stays hidden as it does when Windows starts it.
func launch(dir string) (*instance, error) {
	bin := filepath.Join(dir, "althea.exe")
	data := filepath.Join(dir, "data")
	if err := os.RemoveAll(data); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(data, 0o700); err != nil {
		return nil, err
	}
	// Never at login: a copy in a temporary folder must not add itself to Windows' startup,
	// which the antivirus rightly takes for malware settling in.
	if err := os.WriteFile(filepath.Join(data, "settings.toml"), []byte("[general]\nstart_at_login = false\n"), 0o600); err != nil {
		return nil, err
	}
	db, err := store.Open(context.Background(), filepath.Join(data, "althea.db"))
	if err != nil {
		return nil, err
	}
	err = db.KV("althea.onboarding").Set(context.Background(), "done", []byte("1"))
	if cerr := db.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	in := &instance{dir: dir, cmd: exec.CommandContext(context.Background(), bin, "--mygo-opened-at-login"), exited: make(chan error, 1)} //nolint:gosec // the build being measured
	in.started = time.Now()
	if err := in.cmd.Start(); err != nil {
		return nil, err
	}
	go func() { in.exited <- in.cmd.Wait() }()
	return in, nil
}

var sinceStart = regexp.MustCompile(`msg=ready .*since_start=(\S+)`)

// ready waits for the log's ready line, and returns when it came and what Althea measured.
func (in *instance) ready(timeout time.Duration) (wall, inside time.Duration, err error) {
	log := filepath.Join(in.dir, "data", "logs", "althea.log")
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-in.exited:
			return 0, 0, fmt.Errorf("althea exited before it was ready: %w", err)
		default:
		}
		if data, err := os.ReadFile(log); err == nil {
			if m := sinceStart.FindSubmatch(data); m != nil {
				wall = time.Since(in.started)
				inside, err = time.ParseDuration(string(m[1]))
				return wall, inside, err
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	return 0, 0, errors.New("no ready line in the log")
}

func (in *instance) stop() {
	_ = in.cmd.Process.Kill()
	select {
	case err := <-in.exited:
		_ = err // killed: it exits with an error
	case <-time.After(10 * time.Second):
	}
}

func runMeasure(exe string, count int, idle time.Duration, hotkey bool) error {
	dir, err := install(exe)
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	// A first start, not counted, as after installing.
	warm, err := launch(dir)
	if err != nil {
		return err
	}
	_, _, err = warm.ready(30 * time.Second)
	warm.stop()
	if err != nil {
		return err
	}
	var walls, insides, mems, cpus []float64
	var shows []float64
	var shown resources
	for i := range count {
		in, err := launch(dir)
		if err != nil {
			return err
		}
		wall, inside, err := in.ready(30 * time.Second)
		if err != nil {
			in.stop()
			return err
		}
		time.Sleep(idle)
		pid := uint32(in.cmd.Process.Pid) //nolint:gosec // a process ID fits
		cpu0, _ := cpuTime(pid)
		time.Sleep(10 * time.Second)
		cpu1, _ := cpuTime(pid)
		r, err := sample(pid)
		if err != nil {
			in.stop()
			return err
		}
		if hotkey && i == 0 {
			shows, err = measureHotkey(pid, 30)
			if err != nil {
				in.stop()
				return err
			}
			// Idle again, after the launcher showed: the memory a user's Althea sits at.
			time.Sleep(idle)
			if shown, err = sample(pid); err != nil {
				in.stop()
				return err
			}
		}
		in.stop()
		walls, insides = append(walls, ms(wall)), append(insides, ms(inside))
		mems = append(mems, float64(r.privateWS)/1e6)
		cpus = append(cpus, 100*(cpu1-cpu0).Seconds()/10)
		fmt.Fprintf(os.Stderr, "run %d: ready in %.0f ms (%.0f ms inside), %.1f MB idle, %.2f %% CPU idle, %d handles, %d GDI\n",
			i+1, ms(wall), ms(inside), float64(r.privateWS)/1e6, cpus[len(cpus)-1], r.handles, r.gdi)
	}
	fmt.Printf("cold start to ready: median %.0f ms (budget 300 ms); inside Althea %.0f ms\n", median(walls), median(insides))
	fmt.Printf("idle memory: median %.1f MB private working set (budget 60 MB)\n", median(mems))
	fmt.Printf("idle CPU: median %.2f %% over 10 s (budget ~0 %%)\n", median(cpus))
	if len(shows) > 0 {
		fmt.Printf("hotkey to visible window: p50 %.1f ms, p95 %.1f ms (budget 50 ms p95)\n", percentile(shows, 50), percentile(shows, 95))
		fmt.Printf("idle memory after showing it 30 times: %.1f MB private working set (budget 60 MB), %d handles, %d GDI\n",
			float64(shown.privateWS)/1e6, shown.handles, shown.gdi)
	}
	return nil
}

// measureHotkey presses Alt+Space n times, each time timing until the launcher shows, then
// hides it with Escape.
func measureHotkey(pid uint32, n int) ([]float64, error) {
	var out []float64
	for range n {
		start := time.Now()
		pressHotkey()
		if !waitVisible(pid, true) {
			return nil, errors.New("the launcher did not show on Alt+Space")
		}
		out = append(out, ms(time.Since(start)))
		time.Sleep(150 * time.Millisecond)
		pressEscape()
		if !waitVisible(pid, false) {
			return nil, errors.New("the launcher did not hide on Escape")
		}
		time.Sleep(300 * time.Millisecond)
	}
	return out, nil
}

func waitVisible(pid uint32, want bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if launcherVisible(pid) == want {
			return true
		}
		time.Sleep(500 * time.Microsecond)
	}
	return false
}

func runSoak(exe string, total, every, toggle time.Duration, csvPath string) error {
	dir, err := install(exe)
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	in, err := launch(dir)
	if err != nil {
		return err
	}
	defer in.stop()
	if _, _, err := in.ready(30 * time.Second); err != nil {
		return err
	}
	f, err := os.Create(csvPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"minutes", "private_ws_mb", "private_bytes_mb", "handles", "gdi", "user", "threads"})
	pid := uint32(in.cmd.Process.Pid) //nolint:gosec // a process ID fits
	var samples []resources
	end := time.Now().Add(total)
	nextSample, nextToggle := time.Now(), time.Now().Add(toggle)
	for time.Now().Before(end) {
		select {
		case err := <-in.exited:
			return fmt.Errorf("althea exited during the soak: %w", err)
		default:
		}
		now := time.Now()
		if !now.Before(nextToggle) {
			pressHotkey()
			waitVisible(pid, true)
			time.Sleep(500 * time.Millisecond)
			pressEscape()
			waitVisible(pid, false)
			nextToggle = now.Add(toggle)
		}
		if !now.Before(nextSample) {
			r, err := sample(pid)
			if err != nil {
				return err
			}
			r.at = time.Since(in.started)
			samples = append(samples, r)
			_ = w.Write([]string{
				strconv.FormatFloat(r.at.Minutes(), 'f', 1, 64),
				strconv.FormatFloat(float64(r.privateWS)/1e6, 'f', 1, 64),
				strconv.FormatFloat(float64(r.privateBytes)/1e6, 'f', 1, 64),
				strconv.Itoa(int(r.handles)), strconv.Itoa(int(r.gdi)), strconv.Itoa(int(r.user)), strconv.Itoa(r.threads),
			})
			w.Flush()
			nextSample = now.Add(every)
		}
		time.Sleep(100 * time.Millisecond)
	}
	report(samples)
	return w.Error()
}

// report compares the first sample after warm-up with the last.
func report(samples []resources) {
	if len(samples) < 3 {
		fmt.Println("too few samples to compare")
		return
	}
	first, last := samples[len(samples)/10], samples[len(samples)-1]
	fmt.Printf("from %.0f to %.0f minutes:\n", first.at.Minutes(), last.at.Minutes())
	fmt.Printf("  private working set %.1f → %.1f MB\n", float64(first.privateWS)/1e6, float64(last.privateWS)/1e6)
	fmt.Printf("  private bytes       %.1f → %.1f MB\n", float64(first.privateBytes)/1e6, float64(last.privateBytes)/1e6)
	fmt.Printf("  handles %d → %d, GDI objects %d → %d, USER objects %d → %d, threads %d → %d\n",
		first.handles, last.handles, first.gdi, last.gdi, first.user, last.user, first.threads, last.threads)
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func median(v []float64) float64 { return percentile(v, 50) }

func percentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := slices.Clone(v)
	slices.Sort(s)
	i := int(float64(len(s)-1) * p / 100)
	return s[i]
}
