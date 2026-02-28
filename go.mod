module monitor_cpu

go 1.26.0

require (
	github.com/shirou/gopsutil/v3 v3.24.5
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	golang.org/x/sys v0.29.0 // indirect
)

replace github.com/shirou/gopsutil/v3 => /Users/danielthiem/go/pkg/mod/github.com/shirou/gopsutil/v3@v3.24.5

replace golang.org/x/sys => /Users/danielthiem/go/pkg/mod/golang.org/x/sys@v0.29.0

replace github.com/tklauser/go-sysconf => /Users/danielthiem/go/pkg/mod/github.com/tklauser/go-sysconf@v0.3.12

replace github.com/tklauser/numcpus => /Users/danielthiem/go/pkg/mod/github.com/tklauser/numcpus@v0.6.1

replace github.com/shoenig/go-m1cpu => /Users/danielthiem/go/pkg/mod/github.com/shoenig/go-m1cpu@v0.1.6
