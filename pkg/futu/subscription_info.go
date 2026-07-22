package futu

import (
	"context"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
)

// SubscriptionQuota summarizes OpenD subscription usage. TotalUsed and
// Remaining cover all OpenD connections; OwnUsed covers this Exchange's
// connection only.
type SubscriptionQuota struct {
	TotalUsed int
	Remaining int
	OwnUsed   int
}

func (e *Exchange) QuerySubscriptionQuota(ctx context.Context) (SubscriptionQuota, error) {
	var result SubscriptionQuota
	err := e.withRetryingClient(ctx, func(client *opend.Client) error {
		info, err := client.GetSubInfo(ctx, true)
		if err != nil {
			return err
		}
		result.TotalUsed = int(info.GetTotalUsedQuota())
		result.Remaining = int(info.GetRemainQuota())
		for _, connection := range info.GetConnSubInfoList() {
			if connection.GetIsOwnConnData() {
				result.OwnUsed += int(connection.GetUsedQuota())
			}
		}
		return nil
	})
	return result, err
}
