package vmmanager6

import (
        "crypto/tls"
        "fmt"
        "os"
        "strconv"
        "strings"
        "sync"

        vm6api "../vmmanager6-api-go"
        "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type providerConfiguration struct {
        Client                             *vm6api.Client
        MaxParallel                        int
        CurrentParallel                    int
        MaxVMID                            int
        Mutex                              *sync.Mutex
        Cond                               *sync.Cond
        LogFile                            string
        LogLevels                          map[string]string
        DangerouslyIgnoreUnknownAttributes bool
}

// Provider - Terrafrom properties for vmmanager6
func Provider() *schema.Provider {
	return &schema.Provider{

                Schema: map[string]*schema.Schema{
                        "pm_email": {
                                Type:        schema.TypeString,
                                Optional:    true,
                                DefaultFunc: schema.EnvDefaultFunc("PM_EMAIL", nil),
                                Description: "Email e.g. admin@example.com",
                        },
			"pm_password": {
                                Type:        schema.TypeString,
                                Optional:    true,
                                DefaultFunc: schema.EnvDefaultFunc("PM_PASS", nil),
                                Description: "Password to authenticate into vmmanager6",
                                Sensitive:   true,
                        },
			"pm_api_url": {
                                Type:        schema.TypeString,
                                Required:    true,
                                DefaultFunc: schema.EnvDefaultFunc("PM_API_URL", nil),
                                Description: "https://host.fqdn/vm/v3",
                        },
			"pm_api_token_id": {
                                Type:        schema.TypeString,
                                Optional:    true,
                                DefaultFunc: schema.EnvDefaultFunc("PM_API_TOKEN", nil),
                                Description: "API Token",
                        },
			"pm_parallel": {
                                Type:     schema.TypeInt,
                                Optional: true,
                                Default:  4,
                        },
			"pm_tls_insecure": {
                                Type:        schema.TypeBool,
                                Optional:    true,
                                DefaultFunc: schema.EnvDefaultFunc("PM_TLS_INSECURE", true), //we assume it's a lab!
                                Description: "By default, every TLS connection is verified to be secure. This option allows terraform to proceed and operate on servers considered insecure. For example if you're connecting to a remote host and you do not have the CA cert that issued the VMmanager 6 api url's certificate.",
                        },
			"pm_log_enable": {
                                Type:        schema.TypeBool,
                                Optional:    true,
                                Default:     false,
                                Description: "Enable provider logging to get VMmanager API logs",
                        },
			"pm_log_levels": {
                                Type:        schema.TypeMap,
                                Optional:    true,
                                Description: "Configure the logging level to display; trace, debug, info, warn, etc",
                        },
                        "pm_log_file": {
                                Type:        schema.TypeString,
                                Optional:    true,
                                Default:     "terraform-plugin-vmmanager6.log",
                                Description: "Write logs to this specific file",
                        },
			"pm_timeout": {
                                Type:        schema.TypeInt,
                                Optional:    true,
                                DefaultFunc: schema.EnvDefaultFunc("PM_TIMEOUT", defaultTimeout),
                                Description: "How much second to wait for operations for both provider and api-client, default is 300s",
                        },
                        "pm_dangerously_ignore_unknown_attributes": {
                                Type:        schema.TypeBool,
                                Optional:    true,
                                DefaultFunc: schema.EnvDefaultFunc("PM_DANGEROUSLY_IGNORE_UNKNOWN_ATTRIBUTES", false),
                                Description: "By default this provider will exit if an unknown attribute is found. This is to prevent the accidential destruction of VMs or Data when something in the VMmanager 6 API has changed/updated and is not confirmed to work with this provider. Set this to true at your own risk. It may allow you to proceed in cases when the provider refuses to work, but be aware of the danger in doing so.",
                        },
			"pm_debug": {
                                Type:        schema.TypeBool,
                                Optional:    true,
                                DefaultFunc: schema.EnvDefaultFunc("PM_DEBUG", false),
                                Description: "Enable or disable the verbose debug output from VMmanager 6 api",
                        },
		},
		ResourcesMap: map[string]*schema.Resource{
                        "vmmanager6_vm_qemu":  resourceVmQemu(),
                //        "vmmanager6_lxc":      resourceLxc(),
                //        "vmmanager6_lxc_disk": resourceLxcDisk(),
                //        "vmmanager6_pool":     resourcePool(),
                },

                ConfigureFunc: providerConfigure,
        }
}
func providerConfigure(d *schema.ResourceData) (interface{}, error) {
        client, err := getClient(
                d.Get("pm_api_url").(string),
                d.Get("pm_email").(string),
                d.Get("pm_password").(string),
                d.Get("pm_api_token").(string),
                d.Get("pm_tls_insecure").(bool),
                d.Get("pm_timeout").(int),
                d.Get("pm_debug").(bool),
        )
        if err != nil {
                return nil, err
        }

        // look to see what logging we should be outputting according to the provider configuration
        logLevels := make(map[string]string)
        for logger, level := range d.Get("pm_log_levels").(map[string]interface{}) {
                levelAsString, ok := level.(string)
                if ok {
                        logLevels[logger] = levelAsString
                } else {
                        return nil, fmt.Errorf("invalid logging level %v for %v. Be sure to use a string", level, logger)
                }
        }

        // actually configure logging
        // note that if enable is false here, the configuration will squash all output
        ConfigureLogger(
                d.Get("pm_log_enable").(bool),
                d.Get("pm_log_file").(string),
                logLevels,
        )

        var mut sync.Mutex
        return &providerConfiguration{
                Client:                             client,
                MaxParallel:                        d.Get("pm_parallel").(int),
                CurrentParallel:                    0,
                MaxVMID:                            -1,
                Mutex:                              &mut,
                Cond:                               sync.NewCond(&mut),
                LogFile:                            d.Get("pm_log_file").(string),
                LogLevels:                          logLevels,
                DangerouslyIgnoreUnknownAttributes: d.Get("pm_dangerously_ignore_unknown_attributes").(bool),
        }, nil
}
