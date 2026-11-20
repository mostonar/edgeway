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
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// ANSI color escape codes
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorBold   = "\033[1m"
)

// FormatHeader formats command headers in bold blue
func FormatHeader(msg string) string {
	return fmt.Sprintf("%s%s%s%s", ColorBold, ColorBlue, msg, ColorReset)
}

// FormatStep formats execution steps in cyan
func FormatStep(msg string) string {
	return fmt.Sprintf("%s%s%s", ColorCyan, msg, ColorReset)
}

// FormatWarning formats warning messages in yellow
func FormatWarning(msg string) string {
	return fmt.Sprintf("%s%s%s", ColorYellow, msg, ColorReset)
}

// FormatError formats error messages in bold red
func FormatError(msg string) string {
	return fmt.Sprintf("%s%s%s%s", ColorBold, ColorRed, msg, ColorReset)
}

// FormatSuccess formats success status in bold green
func FormatSuccess(msg string) string {
	return fmt.Sprintf("%s%s%s%s", ColorBold, ColorGreen, msg, ColorReset)
}

// FormatResult consistent formatting for results to be displayed
func FormatResult(result string, elapsed time.Duration, cobraCmd *cobra.Command) string {
	ret := result

	t, _ := cobraCmd.Flags().GetBool("time")
	if t {
		ret += (" [" + elapsed.Round(time.Millisecond).String() + "]")
	}

	return ret
}
