package csi

import (
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-k8s-tester/internal/csi"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func newTest() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run CSI tests",
	}

	cmd.PersistentFlags().BoolVar(&terminateOnExit, "terminate-on-exit", true, "true to terminate EC2 instance on test exit")
	cmd.PersistentFlags().StringVar(&prNum, "pr-num", "", "CSI PR number to check out from kubernetes-sigs/aws-ebs-csi-driver")
	cmd.PersistentFlags().StringVar(&githubAccount, "github-account", "kubernetes-sigs", "GitHub account with aws-ebs-csi-driver repo to use")
	cmd.PersistentFlags().StringVar(&githubBranch, "github-branch", "master", "Branch of aws-ebs-csi-driver repo belonging to --github-account")
	cmd.PersistentFlags().DurationVar(&timeout, "timeout", 20*time.Minute, "e2e test timeout")
	cmd.PersistentFlags().StringVar(&vpcID, "vpc-id", "vpc-0c59620d91b2e1f92", "existing VPC ID to use (provided default VPC ID belongs to aws-k8s-tester test account, leave empty to create a new one)")
	cmd.PersistentFlags().BoolVar(&journalctlLogs, "journalctl-logs", false, "true to get journalctl logs from EC2 instance")

	cmd.AddCommand(
		newTestIntegration(),
	)
	return cmd
}

var (
	terminateOnExit bool
	prNum           string
	githubAccount   string
	githubBranch    string
	timeout         time.Duration
	vpcID           string
	journalctlLogs  bool
)

/*
go install -v ./cmd/aws-k8s-tester

aws-k8s-tester csi test integration \
  --terminate-on-exit=true \
  --github-account=kubernetes-sigs \
  --github-branch=master \
  --timeout=20m
*/

func newTestIntegration() *cobra.Command {
	return &cobra.Command{
		Use:   "integration",
		Short: "Run CSI integration tests without container and Kubernetes",
		Run:   testIntegrationFunc,
	}
}

func testIntegrationFunc(cmd *cobra.Command, args []string) {
	if timeout == time.Duration(0) {
		fmt.Fprintf(os.Stderr, "no timeout specified (%q)\n", timeout)
		os.Exit(1)
	}

	lg, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger (%v)\n", err)
		os.Exit(1)
	}
	lg.Info(
		"starting CSI integration tests",
		zap.String("csi", prNum),
		zap.String("github-account", githubAccount),
		zap.String("github-branch", githubBranch),
		zap.Duration("timeout", timeout),
	)
	cfg, err := csi.CreateConfig(vpcID, prNum, githubAccount, githubBranch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error while config: (%v)\n", err)
		os.Exit(1)
	}

	ct, err := csi.NewTester(cfg, terminateOnExit, journalctlLogs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error while creating new tester: (%v)\n", err)
		os.Exit(1)
	}

	if err = ct.RunIntegration(); err != nil {
		fmt.Fprintf(os.Stderr, "error with CSI integration test (%v)\n", err)
		os.Exit(1)
	}
}
