// Command althea is the Althea launcher.
package main

import (
	"fmt"
	"os"

	"github.com/JoaoVitorOli/althea/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "althea:", err)
		os.Exit(1)
	}
}
