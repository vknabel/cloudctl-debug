package cmd

import (
	"fmt"
	"log"

	"git.f-i-ts.de/cloud-native/cloudctl/cmd/helper"
	output "git.f-i-ts.de/cloud-native/cloudctl/cmd/output"
	"git.f-i-ts.de/cloud-native/cloudctl/pkg"
	"git.f-i-ts.de/cloud-native/cloudctl/pkg/api"
	g "git.f-i-ts.de/cloud-native/cloudctl/pkg/gardener"
	metalgo "github.com/metal-pod/metal-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	clusterCmd = &cobra.Command{
		Use:   "cluster",
		Short: "manage clusters",
		Long:  "TODO",
	}
	clusterCreateCmd = &cobra.Command{
		Use:   "create",
		Short: "create a cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			initGardener()
			return clusterCreate()
		},
		PreRun: bindPFlags,
	}

	clusterListCmd = &cobra.Command{
		Use:     "list",
		Short:   "list clusters",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			initGardener()
			return clusterList()
		},
		PreRun: bindPFlags,
	}
	clusterDeleteCmd = &cobra.Command{
		Use:     "delete <uid>",
		Short:   "delete a cluster",
		Aliases: []string{"rm"},
		RunE: func(cmd *cobra.Command, args []string) error {
			initGardener()
			return clusterDelete(args)
		},
		PreRun: bindPFlags,
	}
	clusterDescribeCmd = &cobra.Command{
		Use:   "describe <uid>",
		Short: "describe a cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			initGardener()
			return clusterDescribe(args)
		},
		PreRun: bindPFlags,
	}
	clusterCredentialsCmd = &cobra.Command{
		Use:   "credentials <uid>",
		Short: "get cluster credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			initGardener()
			return clusterCredentials(args)
		},
		PreRun: bindPFlags,
	}

	clusterInputsCmd = &cobra.Command{
		Use:   "inputs",
		Short: "get possible cluster inputs like k8s versions, etc.",
		RunE: func(cmd *cobra.Command, args []string) error {
			initGardener()
			return clusterInputs()
		},
		PreRun: bindPFlags,
	}
)

func init() {
	clusterCreateCmd.Flags().StringP("name", "", "", "name of the cluster, max 10 characters. [required]")
	clusterCreateCmd.Flags().StringP("description", "", "", "description of the cluster. [required]")
	clusterCreateCmd.Flags().StringP("purpose", "", "production", "purpose of the cluster, can be one of production|dev|eval.")
	clusterCreateCmd.Flags().StringP("owner", "", "", "owner of the cluster. [required]")
	clusterCreateCmd.Flags().StringP("project", "", "", "project where this cluster should belong to. [required]")
	clusterCreateCmd.Flags().StringP("partition", "", "nbg-w8101", "partition of the cluster. [required]")
	clusterCreateCmd.Flags().StringP("version", "", "1.14.3", "kubernetes version of the cluster. [required]")
	clusterCreateCmd.Flags().IntP("minsize", "", 1, "minimal workers of the cluster.")
	clusterCreateCmd.Flags().IntP("maxsize", "", 1, "maximal workers of the cluster.")
	clusterCreateCmd.Flags().IntP("maxsurge", "", 1, "max number of workers created during a update of the cluster.")
	clusterCreateCmd.Flags().IntP("maxunavailable", "", 1, "max number of workers that can be unavailable during a update of the cluster.")
	clusterCreateCmd.Flags().StringSlice("labels", []string{}, "labels of the cluster")
	clusterCreateCmd.Flags().StringSlice("external-networks", []string{"internet"}, "external networks of the cluster, can be internet,mpls")
	clusterCreateCmd.Flags().BoolP("allowprivileged", "", false, "allow privileged containers the cluster.")

	clusterCreateCmd.MarkFlagRequired("name")
	clusterCreateCmd.MarkFlagRequired("description")
	clusterCreateCmd.MarkFlagRequired("owner")
	clusterCreateCmd.MarkFlagRequired("partition")
	clusterCreateCmd.MarkFlagRequired("project")

	clusterCmd.AddCommand(clusterCreateCmd)
	clusterCmd.AddCommand(clusterListCmd)
	clusterCmd.AddCommand(clusterCredentialsCmd)
	clusterCmd.AddCommand(clusterDeleteCmd)
	clusterCmd.AddCommand(clusterDescribeCmd)
	clusterCmd.AddCommand(clusterInputsCmd)
}

func initGardener() {
	var err error
	gardener, err = g.NewGardener(kubeconfig)
	if err != nil {
		log.Fatal(err)
	}
}

func clusterCreate() error {
	owner := viper.GetString("owner")
	name := viper.GetString("name")
	desc := viper.GetString("description")
	purpose := viper.GetString("purpose")
	partition := viper.GetString("partition")
	project := viper.GetString("project")
	// FIXME helper and validation
	networks := viper.GetStringSlice("external-networks")

	nar := metalgo.NetworkAcquireRequest{
		Description: desc,
		Name:        name,
		PartitionID: partition,
		ProjectID:   project,
		// Labels map[string]string `json:"labels"`
	}
	nw, err := metal.NetworkAcquire(&nar)
	if err != nil {
		return err
	}

	nodeNetwork := nw.Network

	if len(nodeNetwork.Prefixes) != 1 {
		return fmt.Errorf("node network creation failed, no or more than one entry for prefixes was/were acquired")
	}

	scr := &api.ShootCreateRequest{
		CreatedBy:            owner, // FIXME from token
		Tenant:               owner, // FIXME from token
		Owner:                owner,
		ProjectID:            project,
		Name:                 name,
		Description:          &desc,
		Purpose:              &purpose,
		LoadBalancerProvider: api.DefaultLoadBalancerProvider,
		MachineImage:         api.DefaultMachineImage,
		FirewallImage:        api.DefaultFirewallImage,
		FirewallSize:         pkg.DefaultMachineTypesOfPartition[partition]["firewall"],
		Workers: []api.Worker{
			{
				Name:           "default-worker",
				MachineType:    pkg.DefaultMachineTypesOfPartition[partition]["worker"],
				AutoScalerMin:  viper.GetInt("minsize"),
				AutoScalerMax:  viper.GetInt("maxsize"),
				MaxSurge:       viper.GetInt("maxsurge"),
				MaxUnavailable: viper.GetInt("maxunavailable"),
				VolumeType:     api.DefaultVolumeType,
				VolumeSize:     api.DefaultVolumeSize,
			},
		},
		Kubernetes: api.Kubernetes{
			AllowPrivilegedContainers: viper.GetBool("allowprivileged"),
			Version:                   viper.GetString("version"),
		},
		Maintenance: api.Maintenance{
			AutoUpdate: &api.MaintenanceAutoUpdate{
				KubernetesVersion: false,
				MachineImage:      false,
			},
			TimeWindow: &api.MaintenanceTimeWindow{
				Begin: "220000+0100",
				End:   "233000+0100",
			},
		},
		NodeNetwork:        nodeNetwork.Prefixes[0],
		AdditionalNetworks: networks,
		Zones:              []string{partition},
	}

	shoot, err := gardener.CreateShoot(scr)
	if err != nil {
		return err
	}
	return printer.Print(shoot)
}

func clusterList() error {
	shoots, err := gardener.ListShoots()
	if err != nil {
		return err
	}
	return printer.Print(shoots)
}
func clusterCredentials(args []string) error {
	ci, err := clusterID("credentials", args)
	if err != nil {
		return err
	}
	credentials, err := gardener.ShootCredentials(ci)
	if err != nil {
		return err
	}
	fmt.Println(credentials)
	return nil
}

func clusterDelete(args []string) error {
	ci, err := clusterID("delete", args)
	if err != nil {
		return err
	}
	shoot, err := gardener.GetShoot(ci)
	if err != nil {
		return err
	}
	printer.Print(shoot)
	helper.Prompt("Press Enter to delete above cluster.")
	shoot, err = gardener.DeleteShoot(args[0])
	if err != nil {
		return err
	}
	return printer.Print(shoot)
}
func clusterDescribe(args []string) error {
	ci, err := clusterID("describe", args)
	if err != nil {
		return err
	}
	shoot, err := gardener.GetShoot(ci)
	if err != nil {
		return err
	}
	return output.YAMLPrinter{}.Print(shoot)
}

type inputs struct {
	KubernetesVersions []string
	Partitions         []string
}

func clusterInputs() error {
	sc, err := gardener.ShootConstraints()
	if err != nil {
		return err
	}

	return output.YAMLPrinter{}.Print(sc)
}

func clusterID(verb string, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("cluster %s requires clusterID as argument", verb)
	}
	if len(args) == 1 {
		return args[0], nil
	}
	return "", fmt.Errorf("cluster %s requires exactly one clusterID as argument", verb)
}
