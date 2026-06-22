package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"videohub/internal/models"
)

// Цена просмотров: 100 тенге за каждые полные 1000 просмотров.
const (
	ViewPayoutViews int64 = 1000
	ViewPayoutTenge int64 = 100
)

// Цена премиума и срок действия в днях. Демо-значения.
const (
	PremiumPriceTenge int64 = 990
	PremiumDays             = 30
)

// ErrInsufficientFunds возвращается, когда баланс канала меньше суммы вывода.
var ErrInsufficientFunds = errors.New("insufficient funds")

const txColumns = `id, user_id, channel_id, type, amount, status, description, card_last4, created_at`

func scanTx(row interface{ Scan(...any) error }, t *models.Transaction) error {
	return row.Scan(&t.ID, &t.UserID, &t.ChannelID, &t.Type, &t.Amount,
		&t.Status, &t.Description, &t.CardLast4, &t.CreatedAt)
}

// AddChannelEarning начисляет указанную сумму на баланс канала, владеющего видео.
// Вызывается вместе с RegisterView в одной транзакции — см. videos.go.
// Здесь — отдельный helper для случая, когда нужно начислить вручную.
func (s *Store) AddChannelEarning(ctx context.Context, videoID string, amount int64) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE channels SET
			balance      = balance + $2,
			total_earned = total_earned + $2
		WHERE id = (SELECT channel_id FROM videos WHERE id = $1)`,
		videoID, amount)
	return err
}

// BuyPremium включает премиум на PremiumDays дней (продлевает, если уже был
// активный) и создаёт SUCCESS-транзакцию PREMIUM_PURCHASE.
func (s *Store) BuyPremium(ctx context.Context, userID string, cardLast4 string) (*models.User, *models.Transaction, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	// Если у пользователя ещё активен премиум — продлеваем от текущего конца.
	var u models.User
	if err := scanUser(tx.QueryRow(ctx, `
		UPDATE users SET
			premium       = TRUE,
			premium_until = GREATEST(COALESCE(premium_until, NOW()), NOW())
			                + make_interval(days => $2::int)
		WHERE id = $1
		RETURNING `+userColumns,
		userID, PremiumDays), &u); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}

	description := "Покупка премиум-подписки на " + strconv.Itoa(PremiumDays) + " дней"
	var t models.Transaction
	if err := scanTx(tx.QueryRow(ctx, `
		INSERT INTO transactions(user_id, type, amount, status, description, card_last4)
		VALUES ($1, 'PREMIUM_PURCHASE', $2, 'SUCCESS', $3, $4)
		RETURNING `+txColumns,
		userID, PremiumPriceTenge, description, cardLast4), &t); err != nil {
		return nil, nil, err
	}

	return &u, &t, tx.Commit(ctx)
}

// SetPremium — админ выдаёт или отзывает премиум. Если until=nil, премиум выключается.
func (s *Store) SetPremium(ctx context.Context, userID string, until *time.Time) (*models.User, error) {
	var u models.User
	err := scanUser(s.Pool.QueryRow(ctx, `
		UPDATE users SET
			premium       = $2 IS NOT NULL,
			premium_until = $2
		WHERE id = $1
		RETURNING `+userColumns, userID, until), &u)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	return &u, err
}

// PayoutChannel списывает amount с баланса канала и создаёт CHANNEL_PAYOUT.
// Если баланс меньше amount — ErrInsufficientFunds.
func (s *Store) PayoutChannel(ctx context.Context, channelID, userID string, amount int64, cardLast4 string) (*models.Transaction, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var balance int64
	if err := tx.QueryRow(ctx, `SELECT balance FROM channels WHERE id=$1 FOR UPDATE`, channelID).Scan(&balance); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if balance < amount {
		return nil, ErrInsufficientFunds
	}
	if _, err := tx.Exec(ctx, `UPDATE channels SET balance = balance - $2 WHERE id = $1`, channelID, amount); err != nil {
		return nil, err
	}

	var t models.Transaction
	if err := scanTx(tx.QueryRow(ctx, `
		INSERT INTO transactions(user_id, channel_id, type, amount, status, description, card_last4)
		VALUES ($1, $2, 'CHANNEL_PAYOUT', $3, 'SUCCESS',
		        'Вывод средств на карту •••• ' || $4, $4)
		RETURNING `+txColumns,
		userID, channelID, amount, cardLast4), &t); err != nil {
		return nil, err
	}

	return &t, tx.Commit(ctx)
}

// AdminAdjustBalance корректирует баланс канала; знак суммы определяет направление.
// Пишет транзакцию ADMIN_ADJUSTMENT с указанным комментарием.
func (s *Store) AdminAdjustBalance(ctx context.Context, channelID string, amount int64, comment string) (*models.Transaction, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE channels SET balance = GREATEST(balance + $2, 0) WHERE id=$1`,
		channelID, amount); err != nil {
		return nil, err
	}

	if comment == "" {
		comment = "Ручная корректировка баланса администратором"
	}
	var t models.Transaction
	if err := scanTx(tx.QueryRow(ctx, `
		INSERT INTO transactions(channel_id, type, amount, status, description)
		VALUES ($1, 'ADMIN_ADJUSTMENT', $2, 'SUCCESS', $3)
		RETURNING `+txColumns,
		channelID, amount, comment), &t); err != nil {
		return nil, err
	}
	return &t, tx.Commit(ctx)
}

func (s *Store) ListTransactionsForUser(ctx context.Context, userID string, limit int) ([]models.Transaction, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT `+txColumns+`
		FROM transactions
		WHERE user_id = $1
		   OR channel_id IN (SELECT id FROM channels WHERE owner_id = $1)
		ORDER BY created_at DESC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Transaction, 0)
	for rows.Next() {
		var t models.Transaction
		if err := scanTx(rows, &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) ListAllTransactions(ctx context.Context, limit int) ([]models.Transaction, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT `+txColumns+` FROM transactions ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Transaction, 0)
	for rows.Next() {
		var t models.Transaction
		if err := scanTx(rows, &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) ListPremiumUsers(ctx context.Context) ([]models.User, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+userColumns+`
		FROM users
		WHERE premium = TRUE
		ORDER BY premium_until DESC NULLS LAST`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.User, 0)
	for rows.Next() {
		var u models.User
		if err := scanUser(rows, &u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
