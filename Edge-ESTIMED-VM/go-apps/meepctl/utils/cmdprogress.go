/*
 * Copyright (c) 2022  The AdvantEDGE Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package utils

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

// Lines of tool output kept to explain a failure in non-verbose mode
const progressTailLines = 40

var (
	// docker build --progress=plain, e.g. "#14 [builder 8/10] RUN pip install ..."
	buildStepRegex = regexp.MustCompile(`^#\d+ \[([^\]]+)\] (.*)`)
	// Output of a RUN step, e.g. "#9 12.31 Get:23 http://archive.ubuntu.com ... [1.9 MB]"
	buildOutputRegex = regexp.MustCompile(`^#\d+ \d+\.\d+ (.*\S.*)`)
	// e.g. "#4 sha256:a853f94d2263... 12.58MB / 45.20MB 1.2s"
	buildDownloadRegex = regexp.MustCompile(`^#\d+ sha256:\w+ ([\d.]+\s?[kKMG]?B) / ([\d.]+\s?[kKMG]?B)`)
	// docker push, e.g. "9a9343896f92: Pushing" / "Layer already exists"
	pushLayerRegex = regexp.MustCompile(`^(\w{12}): (\w.*)`)
)

// RunWithProgress runs a docker build/push behind a single step line.
//
// With --verbose the tool's full output is streamed as-is. Otherwise its
// output is captured and summarized: on a terminal the step line updates in
// place with the current build step and layer download progress, then ends
// with the elapsed time. On failure the last lines of output are printed, so
// the error stays visible without --verbose.
func RunWithProgress(cmd *exec.Cmd, step string, cobraCmd *cobra.Command) error {
	if verbose, _ := cobraCmd.Flags().GetBool("verbose"); verbose {
		fmt.Println(FormatStep(step))
		return StreamCmd(cmd, cobraCmd)
	}

	tty := isTerminal(os.Stdout)
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = w, w
	start := time.Now()
	if err := cmd.Start(); err != nil {
		_ = r.Close()
		_ = w.Close()
		return err
	}
	_ = w.Close() // the child holds its own copy; EOF once it exits

	var tail []string
	var current, activity, download string
	lastDraw := time.Time{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := stripCR(scanner.Text())
		tail = append(tail, line)
		if len(tail) > progressTailLines {
			tail = tail[1:]
		}
		if m := buildStepRegex.FindStringSubmatch(line); m != nil {
			// The command only identifies the step; its live output is more useful
			current, activity, download = "["+m[1]+"] "+truncate(m[2], 30), "", ""
		} else if m := buildOutputRegex.FindStringSubmatch(line); m != nil {
			activity = strings.TrimSpace(m[1])
		} else if m := buildDownloadRegex.FindStringSubmatch(line); m != nil {
			download = "↓ " + m[1] + " / " + m[2]
		} else if m := pushLayerRegex.FindStringSubmatch(line); m != nil {
			current, activity, download = m[1]+": "+m[2], "", ""
		}
		if tty && time.Since(lastDraw) > 100*time.Millisecond {
			status := current
			if activity != "" {
				status += " › " + activity
			}
			if download != "" {
				status += "  " + download
			}
			fmt.Print("\r\033[K" + FormatStep(step) + "  " + truncate(status, statusWidth(step)) + "  " + formatDuration(time.Since(start)))
			lastDraw = time.Now()
		}
	}
	_ = r.Close()
	err = cmd.Wait()

	if tty {
		fmt.Print("\r\033[K")
	}
	if err != nil {
		fmt.Println(FormatStep(step) + "  " + FormatError("✗ "+formatDuration(time.Since(start))))
		for _, line := range tail {
			fmt.Println("     " + line)
		}
		return err
	}
	fmt.Println(FormatStep(step) + "  " + FormatSuccess("✓ "+formatDuration(time.Since(start))))
	return nil
}

// statusWidth is the room left on a terminal line after the step label and
// elapsed time, so the live status never wraps (wrapping breaks \r redraws)
func statusWidth(step string) int {
	cols := 100
	if ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ); err == nil && ws.Col > 0 {
		cols = int(ws.Col)
	} else if c, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && c > 0 {
		cols = c
	}
	w := cols - len([]rune(step)) - 12
	if w < 10 {
		w = 10
	}
	return w
}

// stripCR drops carriage-return redraws some tools emit, keeping the last frame
func stripCR(s string) string {
	if i := strings.LastIndex(s, "\r"); i >= 0 {
		return s[i+1:]
	}
	return s
}
