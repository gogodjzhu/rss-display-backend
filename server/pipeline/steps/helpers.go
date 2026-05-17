package steps

import (
	"fmt"
	"os"
	"time"
)

func tempIOPaths(prefix string) (inPath, outPath string, cleanup func()) {
	ts := time.Now().UnixNano()
	inPath = fmt.Sprintf("%s_%s_%d_in.json", os.TempDir(), prefix, ts)
	outPath = fmt.Sprintf("%s_%s_%d_out.json", os.TempDir(), prefix, ts)
	cleanup = func() {
		os.Remove(inPath)
		os.Remove(outPath)
	}
	return
}