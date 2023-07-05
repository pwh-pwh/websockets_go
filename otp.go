package main

import (
	"context"
	"github.com/google/uuid"
	"time"
)

type OTP struct {
	Key     string
	Created time.Time
}

type RetentionMap map[string]OTP

func NewRetentionMap(ctx context.Context, retentPeriod time.Duration) RetentionMap {
	rm := make(RetentionMap)
	go rm.Retention(ctx, retentPeriod)
	return rm
}

func (rm RetentionMap) NewOTP() OTP {
	otp := OTP{
		Key:     uuid.NewString(),
		Created: time.Now(),
	}
	rm[otp.Key] = otp
	return otp
}

func (rm RetentionMap) VerifyOTP(key string) bool {
	if _, ok := rm[key]; !ok {
		return false
	}
	delete(rm, key)
	return true
}

func (rm RetentionMap) Retention(ctx context.Context, retentionPeriod time.Duration) {
	ticket := time.NewTicker(400 * time.Millisecond)
	for {
		select {
		case <-ticket.C:
			for _, otp := range rm {
				if otp.Created.Add(retentionPeriod).Before(time.Now()) {
					delete(rm, otp.Key)
				}
			}
		case <-ctx.Done():
			return
		}
	}
}
