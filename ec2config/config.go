package ec2config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-k8s-tester/ec2config/plugins"
	ec2types "github.com/aws/aws-k8s-tester/pkg/awsapi/ec2"

	gyaml "github.com/ghodss/yaml"
)

// Config defines EC2 configuration.
type Config struct {
	// EnvPrefix is used to update configuration via environmental variables.
	// The default is "AWS_K8S_TESTER_EC2_".
	EnvPrefix string

	// AWSAccountID is the AWS account ID.
	AWSAccountID string `json:"aws-account-id,omitempty"`
	// AWSRegion is the AWS region.
	AWSRegion string `json:"aws-region,omitempty"`

	// LogDebug is true to enable debug level logging.
	LogDebug bool `json:"log-debug"`

	// LogOutputs is a list of log outputs. Valid values are 'default', 'stderr', 'stdout', or file names.
	// Logs are appended to the existing file, if any.
	// Multiple values are accepted. If empty, it sets to 'default', which outputs to stderr.
	// See https://godoc.org/go.uber.org/zap#Open and https://godoc.org/go.uber.org/zap#Config for more details.
	LogOutputs []string `json:"log-outputs,omitempty"`
	// LogOutputToUploadPath is the aws-k8s-tester log file path to upload to cloud storage.
	// Must be left empty.
	// This will be overwritten by cluster name.
	LogOutputToUploadPath       string `json:"log-output-to-upload-path,omitempty"`
	LogOutputToUploadPathBucket string `json:"log-output-to-upload-path-bucket,omitempty"`
	LogOutputToUploadPathURL    string `json:"log-output-to-upload-path-url,omitempty"`
	// UploadTesterLogs is true to auto-upload log files.
	UploadTesterLogs bool `json:"upload-tester-logs"`

	// Tag is the tag used for all cloudformation stacks.
	Tag string `json:"tag,omitempty"`
	// ClusterName is an unique ID for cluster.
	ClusterName string `json:"cluster-name,omitempty"`

	// WaitBeforeDown is the duration to sleep before EC2 tear down.
	// This is for "test".
	WaitBeforeDown time.Duration `json:"wait-before-down,omitempty"`
	// Down is true to automatically tear down EC2 in "test".
	// Note that this is meant to be used as a flag in "test".
	// Deployer implementation should not call "Down" inside "Up" method.
	Down bool `json:"down"`

	// ConfigPath is the configuration file path.
	// If empty, it is autopopulated.
	// Deployer is expected to update this file with latest status,
	// and to make a backup of original configuration
	// with the filename suffix ".backup.yaml" in the same directory.
	ConfigPath       string    `json:"config-path,omitempty"`
	ConfigPathBucket string    `json:"config-path-bucket,omitempty"` // read-only to user
	ConfigPathURL    string    `json:"config-path-url,omitempty"`    // read-only to user
	UpdatedAt        time.Time `json:"updated-at,omitempty"`         // read-only to user

	// ImageID is the Amazon Machine Image (AMI).
	ImageID string `json:"image-id,omitempty"`
	// UserName is the user name used for running init scripts or SSH access.
	UserName string `json:"user-name,omitempty"`
	// Plugins is the list of plugins.
	Plugins []string `json:"plugins,omitempty"`

	// InitScript contains init scripts (run-instance UserData field).
	// Script must be started with "#!/usr/bin/env bash" IF "Plugins" field is not defined.
	// And will be base64-encoded. Do not base64-encode. Just configure as plain-text.
	// Let this "ec2" package base64-encode.
	// Outputs are saved in "/var/log/cloud-init-output.log" in EC2 instance.
	// "tail -f /var/log/cloud-init-output.log" to check the progress.
	// Reference: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/user-data.html.
	// Note that if both "Plugins" and "InitScript" are not empty,
	// "InitScript" field is always appended to the scripts generated by "Plugins" field.
	InitScript string `json:"init-script,omitempty"`
	// InitScriptCreated is true once the init script has been created.
	// This is to prevent redundant init script updates from plugins.
	InitScriptCreated bool `json:"init-script-created"`

	// InstanceType is the instance type.
	InstanceType string `json:"instance-type,omitempty"`
	// Count is the number of EC2 instances to create.
	Count int `json:"count,omitempty"`

	// KeyName is the name of the key pair used for SSH access.
	// Leave empty to create a temporary one.
	KeyName string `json:"key-name,omitempty"`
	// KeyPath is the file path to the private key.
	KeyPath       string `json:"key-path,omitempty"`
	KeyPathBucket string `json:"key-path-bucket,omitempty"`
	KeyPathURL    string `json:"key-path-url,omitempty"`

	// VPCCIDR is the VPC CIDR.
	VPCCIDR string `json:"vpc-cidr"`
	// VPCID is the VPC ID to use.
	// Leave empty to create a temporary one.
	VPCID      string `json:"vpc-id"`
	VPCCreated bool   `json:"vpc-created"`
	// InternetGatewayID is the internet gateway ID.
	InternetGatewayID string `json:"internet-gateway-id,omitempty"`
	// RouteTableIDs is the list of route table IDs.
	RouteTableIDs []string `json:"route-table-ids,omitempty"`

	// SubnetIDs is a list of subnet IDs to use.
	// If empty, it will fetch subnets from a given or created VPC.
	// And randomly assign them to instances.
	SubnetIDs                  []string          `json:"subnet-ids,omitempty"`
	SubnetIDToAvailibilityZone map[string]string `json:"subnet-id-to-availability-zone,omitempty"` // read-only to user

	// IngressCIDRs is a map from TCP port to CIDR to allow via security groups.
	IngressCIDRs map[int64]string `json:"ingress-cidrs,omitempty"`

	// SecurityGroupIDs is the list of security group IDs.
	// Leave empty to create a temporary one.
	SecurityGroupIDs []string `json:"security-group-ids,omitempty"`

	// AssociatePublicIPAddress is true to associate a public IP address.
	AssociatePublicIPAddress bool `json:"associate-public-ip-address"`

	// Instances is a set of EC2 instances created from this configuration.
	Instances map[string]Instance `json:"instances,omitempty"`

	// Wait is true to wait until all EC2 instances are ready.
	Wait bool `json:"wait"`
}

// Instance represents an EC2 instance.
type Instance struct {
	ImageID             string               `json:"image-id,omitempty"`
	InstanceID          string               `json:"instance-id,omitempty"`
	InstanceType        string               `json:"instance-type,omitempty"`
	KeyName             string               `json:"key-name,omitempty"`
	Placement           Placement            `json:"placement,omitempty"`
	PrivateDNSName      string               `json:"private-dns-name,omitempty"`
	PrivateIP           string               `json:"private-ip,omitempty"`
	PublicDNSName       string               `json:"public-dns-name,omitempty"`
	PublicIP            string               `json:"public-ip,omitempty"`
	State               State                `json:"state,omitempty"`
	SubnetID            string               `json:"subnet-id,omitempty"`
	VPCID               string               `json:"vpc-id,omitempty"`
	BlockDeviceMappings []BlockDeviceMapping `json:"block-device-mappings,omitempty"`
	EBSOptimized        bool                 `json:"ebs-optimized"`
	RootDeviceName      string               `json:"root-device-name,omitempty"`
	RootDeviceType      string               `json:"root-device-type,omitempty"`
	SecurityGroups      []SecurityGroup      `json:"security-groups,omitempty"`
	LaunchTime          time.Time            `json:"launch-time,omitempty"`
}

// Placement defines EC2 placement.
type Placement struct {
	AvailabilityZone string `json:"availability-zone,omitempty"`
	Tenancy          string `json:"tenancy,omitempty"`
}

// State defines an EC2 state.
type State struct {
	Code int64  `json:"code,omitempty"`
	Name string `json:"name,omitempty"`
}

// BlockDeviceMapping defines a block device mapping.
type BlockDeviceMapping struct {
	DeviceName string `json:"device-name,omitempty"`
	EBS        EBS    `json:"ebs,omitempty"`
}

// EBS defines an EBS volume.
type EBS struct {
	DeleteOnTermination bool   `json:"delete-on-termination,omitempty"`
	Status              string `json:"status,omitempty"`
	VolumeID            string `json:"volume-id,omitempty"`
}

// SecurityGroup defines a security group.
type SecurityGroup struct {
	GroupName string `json:"group-name,omitempty"`
	GroupID   string `json:"group-id,omitempty"`
}

// NewDefault returns a copy of the default configuration.
func NewDefault() *Config {
	vv := defaultConfig
	return &vv
}

// defaultConfig is the default configuration.
//  - empty string creates a non-nil object for pointer-type field
//  - omitting an entire field returns nil value
//  - make sure to check both
var defaultConfig = Config{
	EnvPrefix: "AWS_K8S_TESTER_EC2_",
	AWSRegion: "us-west-2",

	WaitBeforeDown: 10 * time.Minute,
	Down:           true,

	LogDebug: false,

	// default, stderr, stdout, or file name
	// log file named with cluster name will be added automatically
	LogOutputs:       []string{"stderr"},
	UploadTesterLogs: false,

	// Amazon Linux 2 AMI (HVM), SSD Volume Type
	ImageID:  "ami-061e7ebbc234015fe",
	UserName: "ec2-user",
	Plugins: []string{
		"update-amazon-linux-2",
		"install-go1.11.2",
		"install-docker-amazon-linux-2",
	},

	// Ubuntu Server 16.04 LTS (HVM), SSD Volume Type
	// ImageID: "ami-ba602bc2",
	// UserName: "ubuntu",
	// Plugins: []string{"update-ubuntu"},

	// 4 vCPU, 15 GB RAM
	InstanceType: "m3.xlarge",
	Count:        1,

	AssociatePublicIPAddress: true,

	VPCCIDR: "192.168.0.0/16",
	IngressCIDRs: map[int64]string{
		22: "0.0.0.0/0",
	},

	Wait: false,
}

// UpdateFromEnvs updates fields from environmental variables.
func (cfg *Config) UpdateFromEnvs() error {
	cc := *cfg

	tp, vv := reflect.TypeOf(&cc).Elem(), reflect.ValueOf(&cc).Elem()
	for i := 0; i < tp.NumField(); i++ {
		jv := tp.Field(i).Tag.Get("json")
		if jv == "" {
			continue
		}
		jv = strings.Replace(jv, ",omitempty", "", -1)
		jv = strings.Replace(jv, "-", "_", -1)
		jv = strings.ToUpper(strings.Replace(jv, "-", "_", -1))
		env := cfg.EnvPrefix + jv
		if os.Getenv(env) == "" {
			continue
		}
		sv := os.Getenv(env)

		fieldName := tp.Field(i).Name

		switch vv.Field(i).Type().Kind() {
		case reflect.String:
			vv.Field(i).SetString(sv)

		case reflect.Bool:
			bb, err := strconv.ParseBool(sv)
			if err != nil {
				return fmt.Errorf("failed to parse %q (%q, %v)", sv, env, err)
			}
			vv.Field(i).SetBool(bb)

		case reflect.Int, reflect.Int32, reflect.Int64:
			iv, err := strconv.ParseInt(sv, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse %q (%q, %v)", sv, env, err)
			}
			vv.Field(i).SetInt(iv)

		case reflect.Uint, reflect.Uint32, reflect.Uint64:
			iv, err := strconv.ParseUint(sv, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse %q (%q, %v)", sv, env, err)
			}
			vv.Field(i).SetUint(iv)

		case reflect.Float32, reflect.Float64:
			fv, err := strconv.ParseFloat(sv, 64)
			if err != nil {
				return fmt.Errorf("failed to parse %q (%q, %v)", sv, env, err)
			}
			vv.Field(i).SetFloat(fv)

		case reflect.Map:
			ss := strings.Split(sv, ",")
			switch fieldName {
			case "IngressCIDRs":
				m := reflect.MakeMap(reflect.TypeOf(map[int64]string{}))
				for i := range ss {
					fields := strings.Split(ss[i], "=")
					nv, nerr := strconv.ParseInt(fields[0], 10, 64)
					if nerr != nil {
						return fmt.Errorf("failed to parse IngressTCPPort %s (%v)", fields[0], nerr)
					}
					m.SetMapIndex(reflect.ValueOf(nv), reflect.ValueOf(fields[1]))
				}
				vv.Field(i).Set(m)

			default:
				return fmt.Errorf("parsing field name %q not supported", fieldName)
			}

		case reflect.Slice:
			ss := strings.Split(sv, ",")
			switch fieldName {
			case "Plugins",
				"SubnetIDs",
				"SecurityGroupIDs":
				slice := reflect.MakeSlice(reflect.TypeOf([]string{}), len(ss), len(ss))
				for i := range ss {
					slice.Index(i).SetString(ss[i])
				}
				vv.Field(i).Set(slice)

			default:
				return fmt.Errorf("parsing field name %q not supported", fieldName)
			}

		default:
			return fmt.Errorf("%q (%v) is not supported as an env", env, vv.Field(i).Type())
		}
	}
	*cfg = cc

	return nil
}

// ValidateAndSetDefaults returns an error for invalid configurations.
// And updates empty fields with default values.
// At the end, it writes populated YAML to aws-k8s-tester config path.
func (cfg *Config) ValidateAndSetDefaults() (err error) {
	if len(cfg.LogOutputs) == 0 {
		return errors.New("EKS LogOutputs is not specified")
	}
	if cfg.AWSRegion == "" {
		return errors.New("empty AWSRegion")
	}
	if cfg.UserName == "" {
		return errors.New("empty UserName")
	}
	if cfg.ImageID == "" {
		return errors.New("empty ImageID")
	}

	if len(cfg.Plugins) > 0 && !cfg.InitScriptCreated {
		txt := cfg.InitScript
		cfg.InitScript, err = plugins.Create(cfg.UserName, cfg.Plugins)
		if err != nil {
			return err
		}
		cfg.InitScript += "\n" + txt
		cfg.InitScriptCreated = true
	}

	if cfg.InstanceType == "" {
		return errors.New("empty InstanceType")
	}
	if cfg.Count < 1 {
		return errors.New("unexpected Count")
	}

	if cfg.ClusterName == "" {
		cfg.Tag = genTag()
		cfg.ClusterName = cfg.Tag + "-" + randString(7)
	}

	if cfg.ConfigPath == "" {
		var f *os.File
		f, err = ioutil.TempFile(os.TempDir(), "aws-k8s-tester-ec2config")
		if err != nil {
			return err
		}
		cfg.ConfigPath, _ = filepath.Abs(f.Name())
		f.Close()
		os.RemoveAll(cfg.ConfigPath)
		cfg.ConfigPathBucket = filepath.Join(cfg.ClusterName, "aws-k8s-tester-ec2config.yaml")
		cfg.ConfigPathURL = genS3URL(cfg.AWSRegion, cfg.Tag, cfg.ConfigPathBucket)
	}

	cfg.LogOutputToUploadPath = filepath.Join(os.TempDir(), fmt.Sprintf("%s.log", cfg.ClusterName))
	logOutputExist := false
	for _, lv := range cfg.LogOutputs {
		if cfg.LogOutputToUploadPath == lv {
			logOutputExist = true
			break
		}
	}
	if !logOutputExist {
		// auto-insert generated log output paths to zap logger output list
		cfg.LogOutputs = append(cfg.LogOutputs, cfg.LogOutputToUploadPath)
	}
	cfg.LogOutputToUploadPathBucket = filepath.Join(cfg.ClusterName, "aws-k8s-tester-ec2.log")
	cfg.LogOutputToUploadPathURL = genS3URL(cfg.AWSRegion, cfg.Tag, cfg.LogOutputToUploadPathBucket)

	if cfg.KeyName == "" {
		cfg.KeyName = cfg.ClusterName
		var f *os.File
		f, err = ioutil.TempFile(os.TempDir(), "aws-k8s-tester-ec2.key")
		if err != nil {
			return err
		}
		cfg.KeyPath, _ = filepath.Abs(f.Name())
		f.Close()
		os.RemoveAll(cfg.KeyPath)
		cfg.KeyPathBucket = filepath.Join(cfg.ClusterName, "aws-k8s-tester-ec2.key")
		cfg.KeyPathURL = genS3URL(cfg.AWSRegion, cfg.Tag, cfg.KeyPathBucket)
	}

	if _, ok := ec2types.InstanceTypes[cfg.InstanceType]; !ok {
		return fmt.Errorf("unexpected InstanceType %q", cfg.InstanceType)
	}

	return nil
}

// Load loads configuration from YAML.
//
// Example usage:
//
//	import "github.com/aws/aws-k8s-tester/internal/ec2/config"
//	cfg := config.Load("test.yaml")
//  p, err := cfg.BackupConfig()
//	err = cfg.ValidateAndSetDefaults()
//
// Do not set default values in this function.
// "ValidateAndSetDefaults" must be called separately,
// to prevent overwriting previous data when loaded from disks.
func Load(p string) (cfg *Config, err error) {
	var d []byte
	d, err = ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}
	cfg = new(Config)
	if err = gyaml.Unmarshal(d, cfg); err != nil {
		return nil, err
	}

	if cfg.Instances == nil {
		cfg.Instances = make(map[string]Instance)
	}

	if !filepath.IsAbs(cfg.ConfigPath) {
		cfg.ConfigPath, err = filepath.Abs(p)
		if err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// Sync persists current configuration and states to disk.
func (cfg *Config) Sync() (err error) {
	if !filepath.IsAbs(cfg.ConfigPath) {
		cfg.ConfigPath, err = filepath.Abs(cfg.ConfigPath)
		if err != nil {
			return err
		}
	}
	cfg.UpdatedAt = time.Now().UTC()
	var d []byte
	d, err = gyaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(cfg.ConfigPath, d, 0600)
}

// BackupConfig stores the original aws-k8s-tester configuration
// file to backup, suffixed with ".backup.yaml".
// Otherwise, deployer will overwrite its state back to YAML.
// Useful when the original configuration would be reused
// for other tests.
func (cfg *Config) BackupConfig() (p string, err error) {
	var d []byte
	d, err = ioutil.ReadFile(cfg.ConfigPath)
	if err != nil {
		return "", err
	}
	p = fmt.Sprintf("%s.%X.backup.yaml",
		cfg.ConfigPath,
		time.Now().UTC().UnixNano(),
	)
	return p, ioutil.WriteFile(p, d, 0600)
}
