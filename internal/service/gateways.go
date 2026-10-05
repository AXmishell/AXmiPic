package service

import (
	"log/slog"

	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/payment"
)

// BuildGateways 依据支付配置构建可注册的支付渠道列表。manual 与 mock 始终
// 注册；alipay/wechat/epay 在 enabled 且凭据齐备时注册。它同时供启动时的配置
// 文件与运行时的后台设置使用。
func BuildGateways(baseURL string, cfg config.PaymentConfig, logger *slog.Logger) ([]payment.Gateway, error) {
	gateways := []payment.Gateway{payment.ManualGateway{}, payment.NewMockGateway(baseURL, logger)}
	if cfg.Alipay.Enabled {
		alipay, err := payment.NewAlipayGateway(payment.AlipayOptions{
			AppID:      cfg.Alipay.AppID,
			PrivateKey: cfg.Alipay.PrivateKey,
			PublicKey:  cfg.Alipay.PublicKey,
			GatewayURL: cfg.Alipay.GatewayURL,
		})
		if err != nil {
			return nil, err
		}
		gateways = append(gateways, alipay)
	}
	if cfg.Wechat.Enabled {
		wechat, err := payment.NewWechatGateway(payment.WechatOptions{
			AppID:             cfg.Wechat.AppID,
			MchID:             cfg.Wechat.MchID,
			SerialNo:          cfg.Wechat.SerialNo,
			PrivateKey:        cfg.Wechat.PrivateKey,
			APIv3Key:          cfg.Wechat.APIv3Key,
			PlatformPublicKey: cfg.Wechat.PlatformPublicKey,
			PlatformSerialNo:  cfg.Wechat.PlatformSerialNo,
			GatewayURL:        cfg.Wechat.GatewayURL,
		})
		if err != nil {
			return nil, err
		}
		gateways = append(gateways, wechat)
	}
	if cfg.Epay.Enabled {
		epay, err := payment.NewEpayGateway(payment.EpayOptions{
			PID:        cfg.Epay.PID,
			Key:        cfg.Epay.Key,
			GatewayURL: cfg.Epay.GatewayURL,
			APIURL:     cfg.Epay.APIURL,
			SubmitURL:  cfg.Epay.SubmitURL,
			PayType:    cfg.Epay.PayType,
		})
		if err != nil {
			return nil, err
		}
		gateways = append(gateways, epay)
	}
	return gateways, nil
}
