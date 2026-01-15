package container

import (
	"os/exec"
)

// StopContainer stops a Docker container by ID.
func StopContainer(containerID string) error {
	return exec.Command("docker", "stop", containerID).Run()
}

// RemoveContainer removes a Docker container by ID.
func RemoveContainer(containerID string) error {
	return exec.Command("docker", "rm", containerID).Run()
}

// ForceRemoveContainer force removes a Docker container by ID.
func ForceRemoveContainer(containerID string) error {
	return exec.Command("docker", "rm", "-f", containerID).Run()
}

// RemoveContainersByLabel removes all containers with a given label.
func RemoveContainersByLabel(label string) error {
	// Find all containers with this label (including stopped)
	cmd := exec.Command("docker", "ps", "-aq", "--filter", "label="+label)
	out, err := cmd.Output()
	if err != nil {
		return err
	}

	// Remove each one
	for _, id := range splitLines(string(out)) {
		if id != "" {
			ForceRemoveContainer(id)
		}
	}
	return nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
