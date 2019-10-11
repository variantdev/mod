package shell

import "os"

func (s *Shell) setPath(path string) *Shell {
	site := *s
	exec := site.Exec
	site.Exec = func(cmd *Command) Result {
		newenv := map[string]string{}
		for k, v := range cmd.Env {
			newenv[k] = v
		}
		newenv["PATH"] = path
		newcmd := *cmd
		newcmd.Env = newenv
		return exec(&newcmd)
	}
	return &site
}

func (s *Shell) prependPath(path string) *Shell {
	return s.setPath(path + ":" + os.Getenv("PATH"))
}

func (s *Shell) PrependPaths(dirs map[string]struct{}) *Shell {
	var path string
	for d := range dirs {
		if path == "" {
			path = d
		} else {
			path = d + ":" + path
		}
	}
	return s.prependPath(path)
}
