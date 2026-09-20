package configs

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Beyondtech-ID/boiler-plate-be-api/pkg/helper"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

var Of *Config = &Config{}

type Config struct {
	AppConfig App   `mapstructure:"app"`
	DBConfig  DB    `mapstructure:"db"`
	Cache     Cache `mapstructure:"cache"`
	OTel      OTel  `mapstructure:"otel"`
}

type OTel struct {
	CollectorAddr string `mapstructure:"collectoraddr"`
}

type App struct {
	Name          string `mapstructure:"name"`
	Host          string `mapstructure:"host"`
	Port          string `mapstructure:"port"`
	Version       string `mapstructure:"version"`
	ErrorJsonPath string `mapstructure:"error_json_path"`
	EnvPrefix     string `mapstructure:"env_prefix"`
	Env           string `mapstructure:"env"`
	ServerUrl     string `mapstructure:"server_url"`
}

type DB struct {
	Name              string        `mapstructure:"name"`
	Host              string        `mapstructure:"host"`
	Port              string        `mapstructure:"port"`
	User              string        `mapstructure:"user"`
	Pass              string        `mapstructure:"pass"`
	MaxIdleConnection int           `mapstructure:"max_idle_connection"`
	MaxOpenConnection int           `mapstructure:"max_open_connection"`
	MaxConnLifeTime   time.Duration `mapstructure:"max_conn_life_time"`
}

type Cache struct {
	Address      string        `mapstructure:"address"`
	UserName     string        `mapstructure:"user_name"`
	Password     string        `mapstructure:"password"`
	DB           int           `mapstructure:"db"`
	UsingTls     bool          `mapstructure:"using_tls"`
	TlsCaCert    string        `mapstructure:"tls_ca_cert"`
	TlsCert      string        `mapstructure:"tls_cert"`
	TlsKey       string        `mapstructure:"tls_key"`
	PoolSize     int           `mapstructure:"pool_size"`
	MinIdleConn  int           `mapstructure:"min_idle_conn"`
	DialTimeout  time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	MaxConnAge   time.Duration `mapstructure:"max_conn_age"`
}

var (
	cfg  *Config
	once = sync.Once{}
)

func GetConfig() *Config {
	once.Do(
		func() {
			_ = godotenv.Load(".env")

			v := viper.New()
			v.AutomaticEnv()
			v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

			file := os.Getenv("CONFIG_FILE")
			if !helper.IsEmpty(file) {
				v.SetConfigFile(file)
				_ = v.ReadInConfig()
			}

			cfg = &Config{}

			// Auto-bind env ke semua field
			BindEnvs(v, cfg, "")

			// Unmarshal hasil ke struct
			if err := v.Unmarshal(cfg); err != nil {
				panic(fmt.Errorf("failed to unmarshal config: %w", err))
			}

			Of = cfg
		},
	)

	return cfg
}

func BindEnvs(v *viper.Viper, iface interface{}, prefix string) {
	ifaceVal := reflect.ValueOf(iface)
	if ifaceVal.Kind() == reflect.Ptr {
		ifaceVal = ifaceVal.Elem()
	}
	ifaceType := ifaceVal.Type()

	for i := 0; i < ifaceVal.NumField(); i++ {
		field := ifaceType.Field(i)
		tag := field.Tag.Get("mapstructure")

		if tag == "" || tag == "-" {
			continue
		}

		envKey := tag
		if prefix != "" {
			envKey = prefix + "." + tag
		}

		if field.Type.Kind() == reflect.Struct {
			BindEnvs(v, ifaceVal.Field(i).Interface(), envKey)
		} else {
			v.BindEnv(envKey)
		}
	}
}

// Parse config file value to the config struct
func Parse(v *viper.Viper) error {
	return v.Unmarshal(&Of)
}

// IsProduction determines is current app instance is on production environtment or not.
func IsProduction() bool {
	return strings.EqualFold(Of.AppConfig.Env, "prod") ||
		strings.EqualFold(Of.AppConfig.Env, "production")
}

func findRootDir(targetFile string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(wd, targetFile)); err == nil {
			return wd, nil
		}

		parentDir := filepath.Dir(wd)
		if parentDir == wd {
			break
		}
		wd = parentDir
	}

	return "", fmt.Errorf("%s not found in the directory hierarchy", targetFile)
}
