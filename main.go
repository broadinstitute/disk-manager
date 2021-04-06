package main

import (
	"flag"
	"github.com/broadinstitute/disk-manager/client"
	"github.com/broadinstitute/disk-manager/config"
	"github.com/broadinstitute/disk-manager/disk"
	"github.com/broadinstitute/disk-manager/logs"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/util/homedir"
	"path/filepath"
)

type args struct {
	local      bool
	kubeconfig string
	configFile string
}

func main() {
	args := parseArgs()

	cfg, err := config.Read(args.configFile)
	if err != nil {
		logs.Error.Fatal(err)
	}

	logs.Info.Printf("Building clients...")
	clients, err := client.Build(args.local, args.kubeconfig)
	if err != nil {
		logs.Error.Fatalf("Error building clients: %v, exiting\n", err)
	}

	m, err := disk.NewDiskManager(cfg, clients)
	if err != nil {
		logs.Error.Fatal(err)
	}

	err = m.Run()
	if err != nil {
		logs.Error.Fatal(err)
	}
}

/* Parse command-line arguments */
func parseArgs() *args {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to kubectl config")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to kubeconfig file")
	}
	local := flag.Bool("local", false, "use this flag when running locally (outside of cluster to use local kube config")
	configFile := flag.String("config-file", "/etc/disk-manager/config.yaml", "path to yaml file with disk-manager config")
	flag.Parse()
	return &args{*local, *kubeconfig, *configFile}
}
