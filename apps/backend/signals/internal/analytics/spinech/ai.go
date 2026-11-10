package spinech

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

// A chatter's orders count only from their first chat on, so the assistant isn't credited with what they'd bought before
func (s *Store) AIImpact(ctx context.Context, channelID uuid.UUID, since time.Time, currency string) (analytics.AIImpact, error) {
	counted := "o.order_id != '' AND (NOT a.chatted OR o.at >= a.first_chat)"
	q := fmt.Sprintf(`
		WITH
		  actors AS (
			SELECT %[1]s AS actor,
			  toUInt8(countIf(e.event_name = '%[4]s') > 0) AS chatted,
			  minIf(e.occurred_at, e.event_name = '%[4]s') AS first_chat
			FROM analytics_events AS e FINAL
			WHERE e.channel_id = ? AND e.occurred_at >= ? AND %[2]s AND e.event_name IN ('page_view', '%[4]s') AND %[1]s != ''
			GROUP BY actor
		  ),
		  orders AS (
			SELECT %[1]s AS actor, e.occurred_at AS at,
			  JSONExtractString(e.properties, 'order_id') AS order_id,
			  JSONExtractInt(e.properties, 'total_cents') AS total,
			  JSONExtractString(e.properties, 'currency') AS currency
			FROM analytics_events AS e FINAL
			WHERE e.channel_id = ? AND e.occurred_at >= ? AND e.event_name = '%[5]s' AND %[1]s != ''
		  ),
		  returned AS (
			SELECT DISTINCT JSONExtractString(e.properties, 'order_id') AS order_id
			FROM analytics_events AS e FINAL
			WHERE e.channel_id = ? AND e.occurred_at >= ? AND e.event_name = 'return_requested'
		  )
		SELECT chatted,
		  toInt64(count()), toInt64(countIf(n > 0)), toInt64(sum(n)), toInt64(sum(revenue)), toInt64(sum(revenue_n)), toInt64(sum(returned_n))
		FROM (
		  SELECT a.actor AS actor, a.chatted AS chatted,
			countIf(%[3]s) AS n,
			sumIf(o.total, %[3]s AND o.currency = ?) AS revenue,
			countIf(%[3]s AND o.currency = ?) AS revenue_n,
			countIf(%[3]s AND o.order_id IN (SELECT order_id FROM returned)) AS returned_n
		  FROM actors AS a
		  LEFT JOIN orders AS o ON o.actor = a.actor
		  GROUP BY a.actor, a.chatted
		)
		GROUP BY chatted`, resolvedActor, notBot, counted, signalsapi.EventAssistantTurn, signalsapi.EventOrderPlaced)
	rows, err := s.conn.Query(ctx, q, channelID, since, channelID, since, channelID, since, currency, currency)
	if err != nil {
		return analytics.AIImpact{}, err
	}
	var out analytics.AIImpact
	for rows.Next() {
		var chatted uint8
		var c analytics.AICohort
		if err := rows.Scan(&chatted, &c.People, &c.Buyers, &c.Orders, &c.RevenueCents, &c.RevenueOrders, &c.ReturnedOrders); err != nil {
			rows.Close()
			return analytics.AIImpact{}, err
		}
		if chatted == 1 {
			out.Chatted = c
		} else {
			out.DidNotChat = c
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return analytics.AIImpact{}, err
	}
	err = s.conn.QueryRow(ctx, `
		SELECT toInt64(count()), toInt64(sum(JSONExtractInt(e.properties, 'total_cents')))
		FROM analytics_events AS e FINAL
		WHERE e.channel_id = ? AND e.occurred_at >= ? AND e.event_name = ?
		  AND JSONExtractString(e.properties, 'assisted_by') = ? AND JSONExtractString(e.properties, 'currency') = ?`,
		channelID, since, signalsapi.EventOrderPlaced, signalsapi.AssistedByAgent, currency).Scan(&out.AgentOrders, &out.AgentRevenueCents)
	return out, err
}

// The products assistant turns showed, the most shown first - carted and ordered count the people who did so after a showing
func (s *Store) AIProducts(ctx context.Context, channelID uuid.UUID, since time.Time, limit int) ([]analytics.AIProductStat, error) {
	productsOf := func(event string) string {
		return fmt.Sprintf(`
			SELECT %s AS actor, e.occurred_at AS at, pid
			FROM analytics_events AS e FINAL
			ARRAY JOIN JSONExtract(e.properties, 'product_ids', 'Array(String)') AS pid
			WHERE e.channel_id = ? AND e.occurred_at >= ? AND %s AND e.event_name = '%s' AND pid != '' AND %s != ''`,
			resolvedActor, notBot, event, resolvedActor)
	}
	q := fmt.Sprintf(`
		WITH
		  shown AS (%[1]s),
		  top AS (
			SELECT pid, toInt64(count()) AS turns, toInt64(uniqExact(actor)) AS people
			FROM shown GROUP BY pid ORDER BY turns DESC, pid LIMIT ?
		  ),
		  carts AS (%[2]s),
		  orders AS (%[3]s),
		  carted AS (
			SELECT s.pid AS pid, toInt64(uniqExact(s.actor)) AS n
			FROM shown AS s INNER JOIN carts AS c ON c.actor = s.actor AND c.pid = s.pid
			WHERE s.pid IN (SELECT pid FROM top) AND c.at >= s.at AND c.at <= s.at + INTERVAL %[4]d DAY
			GROUP BY s.pid
		  ),
		  ordered AS (
			SELECT s.pid AS pid, toInt64(uniqExact(s.actor)) AS n
			FROM shown AS s INNER JOIN orders AS o ON o.actor = s.actor AND o.pid = s.pid
			WHERE s.pid IN (SELECT pid FROM top) AND o.at >= s.at AND o.at <= s.at + INTERVAL %[5]d DAY
			GROUP BY s.pid
		  )
		SELECT t.pid, t.turns, t.people, c.n, o.n
		FROM top AS t
		LEFT JOIN carted AS c ON c.pid = t.pid
		LEFT JOIN ordered AS o ON o.pid = t.pid
		ORDER BY t.turns DESC, t.pid`,
		productsOf(signalsapi.EventAssistantTurn), productsOf("cart_add"), productsOf(signalsapi.EventOrderPlaced), signalsapi.AICartWindowDays, signalsapi.AIOrderWindowDays)
	rows, err := s.conn.Query(ctx, q, channelID, since, limit, channelID, since, channelID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []analytics.AIProductStat
	for rows.Next() {
		var p analytics.AIProductStat
		if err := rows.Scan(&p.ProductID, &p.Shown, &p.People, &p.Carted, &p.Ordered); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
