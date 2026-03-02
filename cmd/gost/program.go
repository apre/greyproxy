package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/go-gost/core/auth"
	"github.com/go-gost/core/logger"
	"github.com/go-gost/core/service"
	greywallapi "github.com/go-gost/gost/internal/greywallapi"
	greywallapi_api "github.com/go-gost/gost/internal/greywallapi/api"
	greywallapi_plugins "github.com/go-gost/gost/internal/greywallapi/plugins"
	greywallapi_ui "github.com/go-gost/gost/internal/greywallapi/ui"
	api_service "github.com/go-gost/x/api/service"
	xauth "github.com/go-gost/x/auth"
	"github.com/go-gost/x/config"
	"github.com/go-gost/x/config/loader"
	auth_parser "github.com/go-gost/x/config/parsing/auth"
	"github.com/go-gost/x/config/parsing/parser"
	xmetrics "github.com/go-gost/x/metrics"
	metrics "github.com/go-gost/x/metrics/service"
	"github.com/go-gost/x/registry"
	"github.com/judwhite/go-svc"
	"github.com/spf13/viper"
)

type program struct {
	srvApi       service.Service
	srvMetrics   service.Service
	srvGreywallApi  *greywallapi.Service
	srvProfiling *http.Server

	cancel context.CancelFunc
}

func (p *program) Init(env svc.Environment) error {
	parser.Init(parser.Args{
		CfgFile:     cfgFile,
		Services:    services,
		Nodes:       nodes,
		Debug:       debug,
		Trace:       trace,
		ApiAddr:     apiAddr,
		MetricsAddr: metricsAddr,
	})

	return nil
}

func (p *program) Start() error {
	cfg, err := parser.Parse()
	if err != nil {
		return err
	}

	if outputFormat != "" {
		if err := cfg.Write(os.Stdout, outputFormat); err != nil {
			return err
		}
		os.Exit(0)
	}

	config.Set(cfg)

	if err := loader.Load(cfg); err != nil {
		return err
	}

	if err := p.run(cfg); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.reload(ctx)

	return nil
}

func (p *program) run(cfg *config.Config) error {
	for _, svc := range registry.ServiceRegistry().GetAll() {
		svc := svc
		go func() {
			svc.Serve()
		}()
	}

	if p.srvApi != nil {
		p.srvApi.Close()
		p.srvApi = nil
	}
	if cfg.API != nil {
		s, err := buildApiService(cfg.API)
		if err != nil {
			return err
		}

		p.srvApi = s

		go func() {
			defer s.Close()

			log := logger.Default().WithFields(map[string]any{"kind": "service", "service": "@api"})

			log.Info("listening on ", s.Addr())
			if err := s.Serve(); !errors.Is(err, http.ErrServerClosed) {
				log.Error(err)
			}
		}()
	}

	if p.srvMetrics != nil {
		p.srvMetrics.Close()
		p.srvMetrics = nil
	}
	if cfg.Metrics != nil && cfg.Metrics.Addr != "" {
		s, err := buildMetricsService(cfg.Metrics)
		if err != nil {
			return err
		}

		p.srvMetrics = s

		xmetrics.Enable(true)

		go func() {
			defer s.Close()

			log := logger.Default().WithFields(map[string]any{"kind": "service", "service": "@metrics"})

			log.Info("listening on ", s.Addr())
			if err := s.Serve(); !errors.Is(err, http.ErrServerClosed) {
				log.Error(err)
			}
		}()
	}

	if p.srvProfiling != nil {
		p.srvProfiling.Close()
		p.srvProfiling = nil
	}
	if cfg.Profiling != nil {
		addr := cfg.Profiling.Addr
		if addr == "" {
			addr = ":6060"
		}
		s := &http.Server{
			Addr: addr,
		}
		p.srvProfiling = s

		go func() {
			defer s.Close()

			log := logger.Default().WithFields(map[string]any{"kind": "service", "service": "@profiling"})

			log.Info("listening on ", addr)
			if err := s.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				log.Error(err)
			}
		}()
	}

	// Build and start greywallapi service if configured
	if p.srvGreywallApi == nil {
		if err := p.buildGreywallApiService(); err != nil {
			logger.Default().Warnf("greywallapi: %v", err)
		}
	}

	return nil
}

func (p *program) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}

	for name, srv := range registry.ServiceRegistry().GetAll() {
		srv.Close()
		logger.Default().Debugf("service %s shutdown", name)
	}

	if p.srvApi != nil {
		p.srvApi.Close()
		logger.Default().Debug("service @api shutdown")
	}
	if p.srvMetrics != nil {
		p.srvMetrics.Close()
		logger.Default().Debug("service @metrics shutdown")
	}
	if p.srvProfiling != nil {
		p.srvProfiling.Close()
		logger.Default().Debug("service @profiling shutdown")
	}
	if p.srvGreywallApi != nil {
		p.srvGreywallApi.Close()
		logger.Default().Debug("service @greywallapi shutdown")
	}

	return nil
}

func (p *program) reload(ctx context.Context) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)

	for {
		select {
		case <-c:
			if err := p.reloadConfig(); err != nil {
				logger.Default().Error(err)
			} else {
				logger.Default().Info("config reloaded")
			}

		case <-ctx.Done():
			return
		}
	}
}

func (p *program) reloadConfig() error {
	cfg, err := parser.Parse()
	if err != nil {
		return err
	}
	config.Set(cfg)

	if err := loader.Load(cfg); err != nil {
		return err
	}

	if err := p.run(cfg); err != nil {
		return err
	}

	return nil
}

func buildApiService(cfg *config.APIConfig) (service.Service, error) {
	var authers []auth.Authenticator
	if auther := auth_parser.ParseAutherFromAuth(cfg.Auth); auther != nil {
		authers = append(authers, auther)
	}
	if cfg.Auther != "" {
		authers = append(authers, registry.AutherRegistry().Get(cfg.Auther))
	}

	var auther auth.Authenticator
	if len(authers) > 0 {
		auther = xauth.AuthenticatorGroup(authers...)
	}

	network := "tcp"
	addr := cfg.Addr
	if strings.HasPrefix(addr, "unix://") {
		network = "unix"
		addr = strings.TrimPrefix(addr, "unix://")
	}
	return api_service.NewService(
		network, addr,
		api_service.PathPrefixOption(cfg.PathPrefix),
		api_service.AccessLogOption(cfg.AccessLog),
		api_service.AutherOption(auther),
	)
}

func (p *program) buildGreywallApiService() error {
	// Read greywallapi config from the same config file using viper
	var gaCfg greywallapi.GreywallApiConfig
	if err := viper.UnmarshalKey("greywallapi", &gaCfg); err != nil {
		return nil // No greywallapi section, skip silently
	}
	if gaCfg.Addr == "" {
		return nil // Not configured
	}

	if gaCfg.PathPrefix == "" {
		gaCfg.PathPrefix = "/"
	}
	if gaCfg.DB == "" {
		gaCfg.DB = "./proxy.db"
	}
	if gaCfg.Auther == "" {
		gaCfg.Auther = "auther-0"
	}
	if gaCfg.Admission == "" {
		gaCfg.Admission = "admission-0"
	}
	if gaCfg.Bypass == "" {
		gaCfg.Bypass = "bypass-0"
	}
	if gaCfg.Resolver == "" {
		gaCfg.Resolver = "resolver-0"
	}

	log := logger.Default().WithFields(map[string]any{"kind": "service", "service": "@greywallapi"})

	// Create shared state (this also opens the DB)
	shared := &greywallapi_api.Shared{}

	// Create a temporary service to get DB, cache, bus
	tmpSvc, err := greywallapi.NewService(&gaCfg, nil)
	if err != nil {
		return err
	}

	shared.DB = tmpSvc.DB
	shared.Cache = tmpSvc.Cache
	shared.Bus = tmpSvc.Bus

	// Create and register gost plugins
	autherPlugin := greywallapi_plugins.NewAuther()
	admissionPlugin := greywallapi_plugins.NewAdmission()
	bypassPlugin := greywallapi_plugins.NewBypass(shared.DB, shared.Cache, shared.Bus)
	resolverPlugin := greywallapi_plugins.NewResolver(shared.Cache)

	registry.AutherRegistry().Register(gaCfg.Auther, autherPlugin)
	registry.AdmissionRegistry().Register(gaCfg.Admission, admissionPlugin)
	registry.BypassRegistry().Register(gaCfg.Bypass, bypassPlugin)
	registry.ResolverRegistry().Register(gaCfg.Resolver, resolverPlugin)

	log.Infof("plugins registered: auther=%s admission=%s bypass=%s resolver=%s",
		gaCfg.Auther, gaCfg.Admission, gaCfg.Bypass, gaCfg.Resolver)

	// Build HTTP router with REST API + HTMX UI + WebSocket
	router, g := greywallapi_api.NewRouter(shared, gaCfg.PathPrefix)
	greywallapi_ui.RegisterPageRoutes(g, shared.DB, shared.Bus)
	greywallapi_ui.RegisterHTMXRoutes(g, shared.DB, shared.Bus)

	// Create the actual service
	svc := &greywallapi.Service{}
	*svc = *tmpSvc
	svc.SetHandler(router)

	p.srvGreywallApi = svc

	go func() {
		log.Info("listening on ", svc.Addr())
		if err := svc.Serve(); !errors.Is(err, http.ErrServerClosed) {
			log.Error(err)
		}
	}()

	return nil
}

func buildMetricsService(cfg *config.MetricsConfig) (service.Service, error) {
	auther := auth_parser.ParseAutherFromAuth(cfg.Auth)
	if cfg.Auther != "" {
		auther = registry.AutherRegistry().Get(cfg.Auther)
	}

	network := "tcp"
	addr := cfg.Addr
	if strings.HasPrefix(addr, "unix://") {
		network = "unix"
		addr = strings.TrimPrefix(addr, "unix://")
	}
	return metrics.NewService(
		network, addr,
		metrics.PathOption(cfg.Path),
		metrics.AutherOption(auther),
	)
}
