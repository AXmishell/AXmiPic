package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// AccountRole distinguishes the two account tables.
type AccountRole string

// Account roles, one per table.
const (
	RoleAdmin    AccountRole = "admin"
	RoleCustomer AccountRole = "customer"
)

// Account is a unified view over an admin or a customer record.
type Account struct {
	ID           string
	Username     string
	PasswordHash string
	Role         AccountRole
	Disabled     bool
	UsedBytes    int64
	QuotaBytes   int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// table returns the gorm model backing the given role.
func tableFor(role AccountRole) any {
	if role == RoleAdmin {
		return &Admin{}
	}
	return &Customer{}
}

// CreateAdmin inserts an admin account.
func (r *Repository) CreateAdmin(ctx context.Context, admin *Admin) error {
	if err := r.db.WithContext(ctx).Create(admin).Error; err != nil {
		return fmt.Errorf("store: create admin: %w", err)
	}
	return nil
}

// CreateCustomer inserts a customer account.
func (r *Repository) CreateCustomer(ctx context.Context, customer *Customer) error {
	if err := r.db.WithContext(ctx).Create(customer).Error; err != nil {
		return fmt.Errorf("store: create customer: %w", err)
	}
	return nil
}

// GetAccountByID returns the account with id from the given role's table, or
// ErrNotFound.
func (r *Repository) GetAccountByID(ctx context.Context, role AccountRole, id string) (*Account, error) {
	if role == RoleAdmin {
		var admin Admin
		if err := r.first(ctx, &admin, "id = ?", id); err != nil {
			return nil, fmt.Errorf("store: admin %q: %w", id, err)
		}
		return accountFromAdmin(&admin), nil
	}
	var customer Customer
	if err := r.first(ctx, &customer, "id = ?", id); err != nil {
		return nil, fmt.Errorf("store: customer %q: %w", id, err)
	}
	return accountFromCustomer(&customer), nil
}

// GetAccountByUsername returns the account with username from the given role's
// table, or ErrNotFound. Usernames are unique per table, so admins and customers
// may share a username.
func (r *Repository) GetAccountByUsername(ctx context.Context, role AccountRole, username string) (*Account, error) {
	if role == RoleAdmin {
		var admin Admin
		if err := r.first(ctx, &admin, "username = ?", username); err != nil {
			return nil, fmt.Errorf("store: admin %q: %w", username, err)
		}
		return accountFromAdmin(&admin), nil
	}
	var customer Customer
	if err := r.first(ctx, &customer, "username = ?", username); err != nil {
		return nil, fmt.Errorf("store: customer %q: %w", username, err)
	}
	return accountFromCustomer(&customer), nil
}

// first loads the first matching row into dest, mapping gorm's not-found error
// to ErrNotFound.
func (r *Repository) first(ctx context.Context, dest any, query string, args ...any) error {
	conditions := append([]any{query}, args...)
	err := r.db.WithContext(ctx).First(dest, conditions...).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// ListCustomers returns every customer account ordered by creation time.
func (r *Repository) ListCustomers(ctx context.Context) ([]Account, error) {
	var customers []Customer
	if err := r.db.WithContext(ctx).Order("created_at ASC").Find(&customers).Error; err != nil {
		return nil, fmt.Errorf("store: list customers: %w", err)
	}
	accounts := make([]Account, 0, len(customers))
	for i := range customers {
		accounts = append(accounts, *accountFromCustomer(&customers[i]))
	}
	return accounts, nil
}

// ListAdmins returns every admin account ordered by creation time.
func (r *Repository) ListAdmins(ctx context.Context) ([]Account, error) {
	var admins []Admin
	if err := r.db.WithContext(ctx).Order("created_at ASC").Find(&admins).Error; err != nil {
		return nil, fmt.Errorf("store: list admins: %w", err)
	}
	accounts := make([]Account, 0, len(admins))
	for i := range admins {
		accounts = append(accounts, *accountFromAdmin(&admins[i]))
	}
	return accounts, nil
}

// CountAdmins returns the number of admin accounts.
func (r *Repository) CountAdmins(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Admin{}).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("store: count admins: %w", err)
	}
	return count, nil
}

// CountEnabledAdmins returns the number of enabled admin accounts.
func (r *Repository) CountEnabledAdmins(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Admin{}).
		Where("disabled = ?", false).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("store: count enabled admins: %w", err)
	}
	return count, nil
}

// CountCustomers returns the number of customer accounts.
func (r *Repository) CountCustomers(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Customer{}).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("store: count customers: %w", err)
	}
	return count, nil
}

// UpdateCustomer updates a customer's disabled flag and returns the result.
func (r *Repository) UpdateCustomer(ctx context.Context, id string, update UserUpdate) (*Account, error) {
	if err := r.db.WithContext(ctx).Model(&Customer{}).Where("id = ?", id).Updates(update).Error; err != nil {
		return nil, fmt.Errorf("store: update customer %q: %w", id, err)
	}
	return r.GetAccountByID(ctx, RoleCustomer, id)
}

// UpdateAdmin updates an admin's disabled flag and returns the result.
func (r *Repository) UpdateAdmin(ctx context.Context, id string, update UserUpdate) (*Account, error) {
	if err := r.db.WithContext(ctx).Model(&Admin{}).Where("id = ?", id).Updates(update).Error; err != nil {
		return nil, fmt.Errorf("store: update admin %q: %w", id, err)
	}
	return r.GetAccountByID(ctx, RoleAdmin, id)
}

// ReserveQuota atomically increases a customer's used bytes if it stays within
// quota. It returns false when the quota would be exceeded.
func (r *Repository) ReserveQuota(ctx context.Context, customerID string, amount int64) (bool, error) {
	result := r.db.WithContext(ctx).Model(&Customer{}).
		Where("id = ? AND (quota_bytes = 0 OR used_bytes + ? <= quota_bytes)", customerID, amount).
		UpdateColumn("used_bytes", gorm.Expr("used_bytes + ?", amount))
	if result.Error != nil {
		return false, fmt.Errorf("store: reserve quota for %q: %w", customerID, result.Error)
	}
	return result.RowsAffected == 1, nil
}

// ReleaseQuota decreases a customer's used bytes.
func (r *Repository) ReleaseQuota(ctx context.Context, customerID string, amount int64) error {
	if err := r.db.WithContext(ctx).Model(&Customer{}).
		Where("id = ? AND used_bytes >= ?", customerID, amount).
		UpdateColumn("used_bytes", gorm.Expr("used_bytes - ?", amount)).Error; err != nil {
		return fmt.Errorf("store: release quota for %q: %w", customerID, err)
	}
	return nil
}

// DeleteAccount removes an account from the table backing its role.
func (r *Repository) DeleteAccount(ctx context.Context, role AccountRole, id string) error {
	result := r.db.WithContext(ctx).Delete(tableFor(role), "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("store: delete account %q: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("store: account %q: %w", id, ErrNotFound)
	}
	return nil
}

func accountFromAdmin(admin *Admin) *Account {
	return &Account{
		ID:           admin.ID,
		Username:     admin.Username,
		PasswordHash: admin.PasswordHash,
		Role:         RoleAdmin,
		Disabled:     admin.Disabled,
		CreatedAt:    admin.CreatedAt,
		UpdatedAt:    admin.UpdatedAt,
	}
}

func accountFromCustomer(customer *Customer) *Account {
	return &Account{
		ID:           customer.ID,
		Username:     customer.Username,
		PasswordHash: customer.PasswordHash,
		Role:         RoleCustomer,
		Disabled:     customer.Disabled,
		UsedBytes:    customer.UsedBytes,
		QuotaBytes:   customer.QuotaBytes,
		CreatedAt:    customer.CreatedAt,
		UpdatedAt:    customer.UpdatedAt,
	}
}
