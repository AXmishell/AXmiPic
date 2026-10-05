package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AXmishell/axmipic/internal/config"
)

// paymentSettingKey 是支付设置在数据库中的键。
const paymentSettingKey = "payment"

// ---- 对外 DTO（密钥仅暴露是否已设置）----

// AlipaySettingsDTO 是支付宝渠道的对外表示。
type AlipaySettingsDTO struct {
	Enabled       bool   `json:"enabled"`
	GatewayURL    string `json:"gateway_url"`
	AppID         string `json:"app_id"`
	PrivateKeySet bool   `json:"private_key_set"`
	PublicKeySet  bool   `json:"public_key_set"`
}

// WechatSettingsDTO 是微信支付渠道的对外表示。
type WechatSettingsDTO struct {
	Enabled              bool   `json:"enabled"`
	GatewayURL           string `json:"gateway_url"`
	AppID                string `json:"app_id"`
	MchID                string `json:"mch_id"`
	SerialNo             string `json:"serial_no"`
	PlatformSerialNo     string `json:"platform_serial_no"`
	PrivateKeySet        bool   `json:"private_key_set"`
	APIv3KeySet          bool   `json:"api_v3_key_set"`
	PlatformPublicKeySet bool   `json:"platform_public_key_set"`
}

// EpaySettingsDTO 是易支付渠道的对外表示。
type EpaySettingsDTO struct {
	Enabled    bool   `json:"enabled"`
	PID        string `json:"pid"`
	GatewayURL string `json:"gateway_url"`
	APIURL     string `json:"api_url"`
	SubmitURL  string `json:"submit_url"`
	PayType    string `json:"pay_type"`
	KeySet     bool   `json:"key_set"`
}

// PaymentSettingsDTO 是支付设置的对外表示。
type PaymentSettingsDTO struct {
	DefaultGateway string            `json:"default_gateway"`
	Alipay         AlipaySettingsDTO `json:"alipay"`
	Wechat         WechatSettingsDTO `json:"wechat"`
	Epay           EpaySettingsDTO   `json:"epay"`
}

// ---- 提交输入（密钥为空表示保持不变）----

// AlipaySettingsInput 是支付宝渠道的提交输入。
type AlipaySettingsInput struct {
	Enabled    bool   `json:"enabled"`
	GatewayURL string `json:"gateway_url"`
	AppID      string `json:"app_id"`
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}

// WechatSettingsInput 是微信支付渠道的提交输入。
type WechatSettingsInput struct {
	Enabled           bool   `json:"enabled"`
	GatewayURL        string `json:"gateway_url"`
	AppID             string `json:"app_id"`
	MchID             string `json:"mch_id"`
	SerialNo          string `json:"serial_no"`
	PrivateKey        string `json:"private_key"`
	APIv3Key          string `json:"api_v3_key"`
	PlatformPublicKey string `json:"platform_public_key"`
	PlatformSerialNo  string `json:"platform_serial_no"`
}

// EpaySettingsInput 是易支付渠道的提交输入。
type EpaySettingsInput struct {
	Enabled    bool   `json:"enabled"`
	PID        string `json:"pid"`
	Key        string `json:"key"`
	GatewayURL string `json:"gateway_url"`
	APIURL     string `json:"api_url"`
	SubmitURL  string `json:"submit_url"`
	PayType    string `json:"pay_type"`
}

// PaymentSettingsInput 是保存支付设置的输入。
type PaymentSettingsInput struct {
	DefaultGateway string              `json:"default_gateway"`
	Alipay         AlipaySettingsInput `json:"alipay"`
	Wechat         WechatSettingsInput `json:"wechat"`
	Epay           EpaySettingsInput   `json:"epay"`
}

// ---- 落库形态（敏感字段为密文）----

type storedPayment struct {
	DefaultGateway string       `json:"default_gateway"`
	Alipay         storedAlipay `json:"alipay"`
	Wechat         storedWechat `json:"wechat"`
	Epay           storedEpay   `json:"epay"`
}

type storedAlipay struct {
	Enabled    bool   `json:"enabled"`
	GatewayURL string `json:"gateway_url"`
	AppID      string `json:"app_id"`
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}

type storedWechat struct {
	Enabled           bool   `json:"enabled"`
	GatewayURL        string `json:"gateway_url"`
	AppID             string `json:"app_id"`
	MchID             string `json:"mch_id"`
	SerialNo          string `json:"serial_no"`
	PrivateKey        string `json:"private_key"`
	APIv3Key          string `json:"api_v3_key"`
	PlatformPublicKey string `json:"platform_public_key"`
	PlatformSerialNo  string `json:"platform_serial_no"`
}

type storedEpay struct {
	Enabled    bool   `json:"enabled"`
	PID        string `json:"pid"`
	Key        string `json:"key"`
	GatewayURL string `json:"gateway_url"`
	APIURL     string `json:"api_url"`
	SubmitURL  string `json:"submit_url"`
	PayType    string `json:"pay_type"`
}

// SetPaymentDefaults 设置用于首次初始化与未配置时的兜底支付配置。
func (s *SettingsService) SetPaymentDefaults(cfg config.PaymentConfig) {
	s.mu.Lock()
	s.paymentDefaults = cfg
	s.mu.Unlock()
}

// SetPaymentApplier 安装一个回调，用于在支付设置变化时重建并切换运行中的渠道。
func (s *SettingsService) SetPaymentApplier(fn func(config.PaymentConfig) error) {
	s.mu.Lock()
	s.paymentApplier = fn
	s.mu.Unlock()
}

// ApplyStoredPayment 用当前已加载的支付配置重建运行中的渠道。启动时在安装
// applier 之后调用，使数据库中的设置立即生效。
func (s *SettingsService) ApplyStoredPayment() error {
	s.mu.RLock()
	cfg := s.currentPayment
	applier := s.paymentApplier
	s.mu.RUnlock()
	if applier == nil {
		return nil
	}
	return applier(cfg)
}

// Payment 返回当前支付设置（密钥脱敏）。
func (s *SettingsService) Payment() PaymentSettingsDTO {
	s.mu.RLock()
	cfg := s.currentPayment
	s.mu.RUnlock()
	return paymentToDTO(cfg)
}

// UpdatePayment 校验并保存支付设置，成功后即时切换运行中的渠道。
func (s *SettingsService) UpdatePayment(ctx context.Context, in PaymentSettingsInput) (PaymentSettingsDTO, error) {
	s.mu.RLock()
	existing := s.currentPayment
	applier := s.paymentApplier
	s.mu.RUnlock()

	merged := paymentConfigFromInput(in)
	// 未提供（空）的密钥沿用现有值。
	if merged.Alipay.PrivateKey == "" {
		merged.Alipay.PrivateKey = existing.Alipay.PrivateKey
	}
	if merged.Alipay.PublicKey == "" {
		merged.Alipay.PublicKey = existing.Alipay.PublicKey
	}
	if merged.Wechat.PrivateKey == "" {
		merged.Wechat.PrivateKey = existing.Wechat.PrivateKey
	}
	if merged.Wechat.APIv3Key == "" {
		merged.Wechat.APIv3Key = existing.Wechat.APIv3Key
	}
	if merged.Wechat.PlatformPublicKey == "" {
		merged.Wechat.PlatformPublicKey = existing.Wechat.PlatformPublicKey
	}
	if merged.Epay.Key == "" {
		merged.Epay.Key = existing.Epay.Key
	}
	if err := validatePaymentConfig(merged); err != nil {
		return PaymentSettingsDTO{}, err
	}
	merged.DefaultGateway = normalizeDefaultGateway(merged)
	// 先构建并应用，确保配置可用；失败则不落库。
	if applier != nil {
		if err := applier(merged); err != nil {
			return PaymentSettingsDTO{}, fmt.Errorf("%w: %v", ErrSettingsConfig, err)
		}
	}
	encoded, err := s.encodePayment(merged)
	if err != nil {
		return PaymentSettingsDTO{}, err
	}
	if err := s.repo.SetSetting(ctx, paymentSettingKey, encoded); err != nil {
		return PaymentSettingsDTO{}, err
	}
	s.setCurrentPayment(merged)
	return s.Payment(), nil
}

// setCurrentPayment 更新内存中的支付配置。
func (s *SettingsService) setCurrentPayment(cfg config.PaymentConfig) {
	s.mu.Lock()
	s.currentPayment = cfg
	s.mu.Unlock()
}

// paymentConfigFromInput 把提交输入转换为运行时配置。
func paymentConfigFromInput(in PaymentSettingsInput) config.PaymentConfig {
	return config.PaymentConfig{
		DefaultGateway: strings.TrimSpace(in.DefaultGateway),
		Alipay: config.AlipayConfig{
			Enabled:    in.Alipay.Enabled,
			GatewayURL: strings.TrimSpace(in.Alipay.GatewayURL),
			AppID:      strings.TrimSpace(in.Alipay.AppID),
			PrivateKey: strings.TrimSpace(in.Alipay.PrivateKey),
			PublicKey:  strings.TrimSpace(in.Alipay.PublicKey),
		},
		Wechat: config.WechatConfig{
			Enabled:           in.Wechat.Enabled,
			GatewayURL:        strings.TrimSpace(in.Wechat.GatewayURL),
			AppID:             strings.TrimSpace(in.Wechat.AppID),
			MchID:             strings.TrimSpace(in.Wechat.MchID),
			SerialNo:          strings.TrimSpace(in.Wechat.SerialNo),
			PrivateKey:        strings.TrimSpace(in.Wechat.PrivateKey),
			APIv3Key:          in.Wechat.APIv3Key,
			PlatformPublicKey: strings.TrimSpace(in.Wechat.PlatformPublicKey),
			PlatformSerialNo:  strings.TrimSpace(in.Wechat.PlatformSerialNo),
		},
		Epay: config.EpayConfig{
			Enabled:    in.Epay.Enabled,
			PID:        strings.TrimSpace(in.Epay.PID),
			Key:        strings.TrimSpace(in.Epay.Key),
			GatewayURL: strings.TrimSpace(in.Epay.GatewayURL),
			APIURL:     strings.TrimSpace(in.Epay.APIURL),
			SubmitURL:  strings.TrimSpace(in.Epay.SubmitURL),
			PayType:    strings.TrimSpace(in.Epay.PayType),
		},
	}
}

// validatePaymentConfig 校验支付配置是否可用。
func validatePaymentConfig(cfg config.PaymentConfig) error {
	switch cfg.DefaultGateway {
	case "", "manual", "mock", "alipay", "wechat", "epay":
	default:
		return fmt.Errorf("%w: unknown default gateway %q", ErrSettingsConfig, cfg.DefaultGateway)
	}
	if cfg.Alipay.Enabled {
		if cfg.Alipay.AppID == "" || cfg.Alipay.PrivateKey == "" || cfg.Alipay.PublicKey == "" {
			return fmt.Errorf("%w: alipay app_id, private_key and public_key are required", ErrSettingsConfig)
		}
	}
	if cfg.Wechat.Enabled {
		if cfg.Wechat.AppID == "" || cfg.Wechat.MchID == "" || cfg.Wechat.SerialNo == "" || cfg.Wechat.PrivateKey == "" {
			return fmt.Errorf("%w: wechat app_id, mch_id, serial_no and private_key are required", ErrSettingsConfig)
		}
		if len(cfg.Wechat.APIv3Key) != 32 {
			return fmt.Errorf("%w: wechat api_v3_key must be 32 bytes", ErrSettingsConfig)
		}
	}
	if cfg.Epay.Enabled {
		if cfg.Epay.PID == "" || cfg.Epay.Key == "" || cfg.Epay.GatewayURL == "" {
			return fmt.Errorf("%w: epay pid, key and gateway_url are required", ErrSettingsConfig)
		}
	}
	return nil
}

// normalizeDefaultGateway 在默认渠道未注册（被禁用）时回退到 manual。
func normalizeDefaultGateway(cfg config.PaymentConfig) string {
	registered := map[string]bool{"manual": true, "mock": true}
	if cfg.Alipay.Enabled {
		registered["alipay"] = true
	}
	if cfg.Wechat.Enabled {
		registered["wechat"] = true
	}
	if cfg.Epay.Enabled {
		registered["epay"] = true
	}
	if registered[cfg.DefaultGateway] {
		return cfg.DefaultGateway
	}
	return "manual"
}

// paymentToDTO 组装支付设置的对外表示，敏感字段仅暴露是否已设置。
func paymentToDTO(cfg config.PaymentConfig) PaymentSettingsDTO {
	return PaymentSettingsDTO{
		DefaultGateway: cfg.DefaultGateway,
		Alipay: AlipaySettingsDTO{
			Enabled:       cfg.Alipay.Enabled,
			GatewayURL:    cfg.Alipay.GatewayURL,
			AppID:         cfg.Alipay.AppID,
			PrivateKeySet: cfg.Alipay.PrivateKey != "",
			PublicKeySet:  cfg.Alipay.PublicKey != "",
		},
		Wechat: WechatSettingsDTO{
			Enabled:              cfg.Wechat.Enabled,
			GatewayURL:           cfg.Wechat.GatewayURL,
			AppID:                cfg.Wechat.AppID,
			MchID:                cfg.Wechat.MchID,
			SerialNo:             cfg.Wechat.SerialNo,
			PlatformSerialNo:     cfg.Wechat.PlatformSerialNo,
			PrivateKeySet:        cfg.Wechat.PrivateKey != "",
			APIv3KeySet:          cfg.Wechat.APIv3Key != "",
			PlatformPublicKeySet: cfg.Wechat.PlatformPublicKey != "",
		},
		Epay: EpaySettingsDTO{
			Enabled:    cfg.Epay.Enabled,
			PID:        cfg.Epay.PID,
			GatewayURL: cfg.Epay.GatewayURL,
			APIURL:     cfg.Epay.APIURL,
			SubmitURL:  cfg.Epay.SubmitURL,
			PayType:    cfg.Epay.PayType,
			KeySet:     cfg.Epay.Key != "",
		},
	}
}

// encodePayment 把支付配置序列化，敏感字段以密文写入。
func (s *SettingsService) encodePayment(cfg config.PaymentConfig) (string, error) {
	encrypt := func(value string) (string, error) { return s.cipher.Encrypt(value) }

	alipayPriv, err := encrypt(cfg.Alipay.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("settings: encrypt alipay private key: %w", err)
	}
	alipayPub, err := encrypt(cfg.Alipay.PublicKey)
	if err != nil {
		return "", fmt.Errorf("settings: encrypt alipay public key: %w", err)
	}
	wechatPriv, err := encrypt(cfg.Wechat.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("settings: encrypt wechat private key: %w", err)
	}
	wechatKey, err := encrypt(cfg.Wechat.APIv3Key)
	if err != nil {
		return "", fmt.Errorf("settings: encrypt wechat api_v3_key: %w", err)
	}
	wechatPub, err := encrypt(cfg.Wechat.PlatformPublicKey)
	if err != nil {
		return "", fmt.Errorf("settings: encrypt wechat platform public key: %w", err)
	}
	epayKey, err := encrypt(cfg.Epay.Key)
	if err != nil {
		return "", fmt.Errorf("settings: encrypt epay key: %w", err)
	}

	payload, err := json.Marshal(storedPayment{
		DefaultGateway: cfg.DefaultGateway,
		Alipay: storedAlipay{
			Enabled:    cfg.Alipay.Enabled,
			GatewayURL: cfg.Alipay.GatewayURL,
			AppID:      cfg.Alipay.AppID,
			PrivateKey: alipayPriv,
			PublicKey:  alipayPub,
		},
		Wechat: storedWechat{
			Enabled:           cfg.Wechat.Enabled,
			GatewayURL:        cfg.Wechat.GatewayURL,
			AppID:             cfg.Wechat.AppID,
			MchID:             cfg.Wechat.MchID,
			SerialNo:          cfg.Wechat.SerialNo,
			PrivateKey:        wechatPriv,
			APIv3Key:          wechatKey,
			PlatformPublicKey: wechatPub,
			PlatformSerialNo:  cfg.Wechat.PlatformSerialNo,
		},
		Epay: storedEpay{
			Enabled:    cfg.Epay.Enabled,
			PID:        cfg.Epay.PID,
			Key:        epayKey,
			GatewayURL: cfg.Epay.GatewayURL,
			APIURL:     cfg.Epay.APIURL,
			SubmitURL:  cfg.Epay.SubmitURL,
			PayType:    cfg.Epay.PayType,
		},
	})
	if err != nil {
		return "", fmt.Errorf("settings: encode payment: %w", err)
	}
	return string(payload), nil
}

// decodePayment 解析落库的支付设置并解密敏感字段。
func (s *SettingsService) decodePayment(raw string) (config.PaymentConfig, error) {
	var stored storedPayment
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return config.PaymentConfig{}, fmt.Errorf("settings: decode payment: %w", err)
	}
	decrypt := func(value string) (string, error) {
		plain, err := s.cipher.Decrypt(value)
		if err != nil {
			return "", fmt.Errorf("settings: decrypt payment secret: %w", err)
		}
		return plain, nil
	}
	alipayPriv, err := decrypt(stored.Alipay.PrivateKey)
	if err != nil {
		return config.PaymentConfig{}, err
	}
	alipayPub, err := decrypt(stored.Alipay.PublicKey)
	if err != nil {
		return config.PaymentConfig{}, err
	}
	wechatPriv, err := decrypt(stored.Wechat.PrivateKey)
	if err != nil {
		return config.PaymentConfig{}, err
	}
	wechatKey, err := decrypt(stored.Wechat.APIv3Key)
	if err != nil {
		return config.PaymentConfig{}, err
	}
	wechatPub, err := decrypt(stored.Wechat.PlatformPublicKey)
	if err != nil {
		return config.PaymentConfig{}, err
	}
	epayKey, err := decrypt(stored.Epay.Key)
	if err != nil {
		return config.PaymentConfig{}, err
	}
	return config.PaymentConfig{
		DefaultGateway: stored.DefaultGateway,
		Alipay: config.AlipayConfig{
			Enabled:    stored.Alipay.Enabled,
			GatewayURL: stored.Alipay.GatewayURL,
			AppID:      stored.Alipay.AppID,
			PrivateKey: alipayPriv,
			PublicKey:  alipayPub,
		},
		Wechat: config.WechatConfig{
			Enabled:           stored.Wechat.Enabled,
			GatewayURL:        stored.Wechat.GatewayURL,
			AppID:             stored.Wechat.AppID,
			MchID:             stored.Wechat.MchID,
			SerialNo:          stored.Wechat.SerialNo,
			PrivateKey:        wechatPriv,
			APIv3Key:          wechatKey,
			PlatformPublicKey: wechatPub,
			PlatformSerialNo:  stored.Wechat.PlatformSerialNo,
		},
		Epay: config.EpayConfig{
			Enabled:    stored.Epay.Enabled,
			PID:        stored.Epay.PID,
			Key:        epayKey,
			GatewayURL: stored.Epay.GatewayURL,
			APIURL:     stored.Epay.APIURL,
			SubmitURL:  stored.Epay.SubmitURL,
			PayType:    stored.Epay.PayType,
		},
	}, nil
}
