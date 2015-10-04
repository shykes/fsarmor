package fsarmor

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

func MkAnnotation(target string) string {
	target = TreePath(target)
	if target == "/" {
		return "0"
	}
	return fmt.Sprintf("%d/%s", strings.Count(target, "/")+1, target)
}

func ParseAnnotation(annot string) (target string, err error) {
	annot = TreePath(annot)
	parts := strings.Split(annot, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid annotation path")
	}
	lvl, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return "", err
	}

	if int(lvl) == 0 {
		return "", nil
	}

	if len(parts)-1 != int(lvl) {
		return "", fmt.Errorf("invalid annotation path")
	}

	return path.Join(parts[1:]...), nil
}

func TreePath(p string) string {
	p = path.Clean(p)
	if p == "/" || p == "." {
		return "/"
	}
	// Remove leading / from the path
	// as libgit2.TreeEntryByPath does not accept it
	p = strings.TrimLeft(p, "/")
	return p
}
