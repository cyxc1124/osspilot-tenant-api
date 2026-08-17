package storage

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type STSCreds struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiration      time.Time
}

func SessionToken(ctx context.Context, endpoint, region, accessKey, secretKey string, seconds int32) (STSCreds, error) {
	cli := stsClient(endpoint, region, accessKey, secretKey, "")
	out, err := cli.GetSessionToken(ctx, &sts.GetSessionTokenInput{DurationSeconds: aws.Int32(seconds)})
	if err != nil {
		return STSCreds{}, err
	}
	if out.Credentials == nil || out.Credentials.AccessKeyId == nil || out.Credentials.SecretAccessKey == nil || out.Credentials.SessionToken == nil || out.Credentials.Expiration == nil {
		return STSCreds{}, errors.New("sts_invalid_response")
	}
	return STSCreds{
		AccessKeyID: *out.Credentials.AccessKeyId, SecretAccessKey: *out.Credentials.SecretAccessKey,
		SessionToken: *out.Credentials.SessionToken, Expiration: *out.Credentials.Expiration,
	}, nil
}

func CallerAccount(ctx context.Context, endpoint, region, accessKey, secretKey, session string) (string, error) {
	cli := stsClient(endpoint, region, accessKey, secretKey, session)
	out, err := cli.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", err
	}
	if out.Account != nil && strings.TrimSpace(*out.Account) != "" {
		return strings.TrimSpace(*out.Account), nil
	}
	if out.Arn != nil {
		arn := *out.Arn
		if i := strings.LastIndex(arn, ":user/"); i >= 0 {
			return arn[i+6:], nil
		}
		if strings.HasSuffix(arn, ":root") {
			parts := strings.Split(arn, "::")
			if len(parts) > 0 {
				return strings.TrimSuffix(parts[len(parts)-1], ":root"), nil
			}
		}
	}
	if out.UserId != nil && strings.TrimSpace(*out.UserId) != "" {
		return strings.TrimSpace(*out.UserId), nil
	}
	return "", errors.New("sts caller identity is missing tenant mapping")
}

func stsClient(endpoint, region, accessKey, secretKey, session string) *sts.Client {
	if region == "" {
		region = "us-east-1"
	}
	awscfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(accessKey, secretKey, session),
	}
	return sts.NewFromConfig(awscfg, func(o *sts.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(strings.TrimRight(endpoint, "/"))
		}
	})
}
