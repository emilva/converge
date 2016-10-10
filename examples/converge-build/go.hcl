param "go-version" {
  default = "1.7.1"
}

param "go-sha256sum" {
  default = "43ad621c9b014cde8db17393dc108378d37bc853aa351a6c74bf6432c1bbd182"
}

file.content "go-sha256" {
  destination = "/tmp/go{{param `go-version`}}-sha256sum.txt"
  type        = "file"
  content     = "{{param `go-sha256sum`}} go{{param `go-version`}}-sha256sum.txt"
}

wait.query "curl-check" {
  check        = "which curl"
  interval     = "2s"
  max_retry    = 60
  grace_period = "3s"
  interpreter  = "/bin/bash"
}

task "go-dl" {
  check       = "[[ -f /tmp/go{{param `go-version`}}.linux-amd64.tar.gz ]]"
  apply       = "curl -L -o /tmp/go{{param `go-version`}}.linux-amd64.tar.gz  https://storage.googleapis.com/golang/go{{param `go-version`}}.linux-amd64.tar.gz"
  dir         = "/tmp"
  depends     = ["wait.query.curl-check","file.content.go-sha256"]
  interpreter = "/bin/bash"
}

task "go-checksum" {
  check       = "[[ -f /tmp/go{{param `go-version`}}.linux-amd64.tar.gz ]]"
  apply       = "echo checksum failed"
  dir         = "/tmp"
  depends     = ["file.content.go-sha256", "task.go-dl"]
  interpreter = "/bin/bash"
}

task "go-extract" {
  check       = "[[ -d /usr/local/go ]]"
  apply       = "tar -xvzf /tmp/go{{param `go-version`}}.linux-amd64.tar.gz 2>&1 > /dev/null"
  dir         = "/usr/local"
  depends     = ["task.go-checksum"]
  interpreter = "/bin/bash"
}

file "go-symlink" {
  destination = "/usr/local/bin/go"
  target      = "/usr/local/go/bin/go"
  type        = "symlink"
  state       = "present"
  depends     = ["task.go-extract"]
}