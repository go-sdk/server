package jwtx

import (
	"encoding/json"
	"maps"
	"time"

	"github.com/go-sdk/core/errx"
	"github.com/golang-jwt/jwt/v5"
)

// Time 表示 JWT 声明中的时间，payload 内以秒级时间戳编解码。
type Time struct {
	time.Time
}

// NewTime 包装指定时间为 JWT 声明时间。
func NewTime(t time.Time) Time {
	return Time{Time: t}
}

func (t Time) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.Unix())
}

//goland:noinspection GoMixedReceiverTypes
func (t *Time) UnmarshalJSON(data []byte) error {
	var seconds int64
	if err := json.Unmarshal(data, &seconds); err != nil {
		return errx.Wrap(err, "unmarshal jwt time")
	}
	t.Time = time.Unix(seconds, 0).UTC()
	return nil
}

// MapClaims 与服务端中间件写入上下文的声明类型一致，用于构造或读取任意 payload。
type MapClaims = jwt.MapClaims

// Claims 是带常用标准声明和额外声明的 JWT 载荷：jti/sub/iat/exp 为独立字段，
// 其余声明与标准声明同级扁平存放在 Extra 中，可直接用于 Sign 和 Parse。
type Claims struct {
	ID        string
	Subject   string
	IssuedAt  Time
	ExpiresAt Time
	Extra     map[string]any
}

func (c Claims) MarshalJSON() ([]byte, error) {
	values := map[string]any{}
	maps.Copy(values, c.Extra)
	if c.ID != "" {
		values["jti"] = c.ID
	}
	if c.Subject != "" {
		values["sub"] = c.Subject
	}
	if !c.IssuedAt.IsZero() {
		values["iat"] = c.IssuedAt
	}
	if !c.ExpiresAt.IsZero() {
		values["exp"] = c.ExpiresAt
	}
	return json.Marshal(values)
}

//goland:noinspection GoMixedReceiverTypes
func (c *Claims) UnmarshalJSON(data []byte) error {
	values := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &values); err != nil {
		return errx.Wrap(err, "unmarshal jwt claims")
	}
	c.Extra = nil
	for key, raw := range values {
		var err error
		switch key {
		case "jti":
			err = json.Unmarshal(raw, &c.ID)
		case "sub":
			err = json.Unmarshal(raw, &c.Subject)
		case "iat":
			err = json.Unmarshal(raw, &c.IssuedAt)
		case "exp":
			err = json.Unmarshal(raw, &c.ExpiresAt)
		default:
			var value any
			if err = json.Unmarshal(raw, &value); err == nil {
				if c.Extra == nil {
					c.Extra = map[string]any{}
				}
				c.Extra[key] = value
			}
		}
		if err != nil {
			return errx.Wrap(err, "unmarshal jwt claim "+key)
		}
	}
	return nil
}

// Decode 把额外声明整体解码到目标结构，用于业务侧读取强类型的自定义声明。
func (c Claims) Decode(target any) error {
	encoded, err := json.Marshal(c.Extra)
	if err != nil {
		return errx.Wrap(err, "encode jwt extra claims")
	}
	if err = json.Unmarshal(encoded, target); err != nil {
		return errx.Wrap(err, "decode jwt extra claims")
	}
	return nil
}

// 以下方法实现 golang-jwt 的 Claims 接口，供签发和解析校验使用。
var _ jwt.Claims = (*Claims)(nil)

func (c Claims) GetExpirationTime() (*jwt.NumericDate, error) {
	if c.ExpiresAt.IsZero() {
		return nil, nil
	}
	return jwt.NewNumericDate(c.ExpiresAt.Time), nil
}

func (c Claims) GetIssuedAt() (*jwt.NumericDate, error) {
	if c.IssuedAt.IsZero() {
		return nil, nil
	}
	return jwt.NewNumericDate(c.IssuedAt.Time), nil
}

func (c Claims) GetNotBefore() (*jwt.NumericDate, error) { return nil, nil }

func (c Claims) GetIssuer() (string, error) { return "", nil }

func (c Claims) GetSubject() (string, error) { return c.Subject, nil }

func (c Claims) GetAudience() (jwt.ClaimStrings, error) { return nil, nil }
