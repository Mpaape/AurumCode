package runner

import (
	"os/exec"
)

func Archive(name string) error {
	cmd := exec.Command("sh", "-c", "tar czf out.tgz "+name)
	return cmd.Run()
}
