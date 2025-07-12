package jobs

import (
	"github.com/revaspay/backend/internal/queue"
	"github.com/revaspay/backend/internal/services/kyc"
	"github.com/revaspay/backend/internal/services/payment"
	"github.com/revaspay/backend/internal/services/wallet"
	"gorm.io/gorm"
)

// RegisterAllJobHandlers registers all job handlers with the queue
func RegisterAllJobHandlers(
	q queue.QueueInterface,
	db *gorm.DB,
	paymentSvc *payment.PaymentService,
	walletSvc *wallet.WalletService,
	kycSvc *kyc.KYCService,
) {
	// Register payment webhook job handlers
	RegisterPaymentWebhookJobHandlers(q, db, paymentSvc, walletSvc)

	// Register recurring payment job handlers
	RegisterRecurringPaymentJobHandlers(q, db, paymentSvc, walletSvc)

	// Register withdrawal job handlers
	withdrawalJob := NewWithdrawalJob(db, q, paymentSvc, walletSvc)
	if qAdapter, ok := q.(*queue.QueueAdapter); ok {
		withdrawalJob.RegisterHandlers(qAdapter)
	}

	// Register KYC verification job handlers
	RegisterKYCVerificationJobHandlers(q, db, kycSvc)

	// Register virtual account job handlers
	RegisterVirtualAccountJobHandlers(q, db, paymentSvc, walletSvc)

	// Register referral reward job handlers
	RegisterReferralRewardJobHandlers(q, db, walletSvc)

	// Auto-withdraw job is registered in its constructor
	NewAutoWithdrawJob(db, q)
}

// ScheduleRecurringJobs schedules all recurring jobs
func ScheduleRecurringJobs(
	q queue.QueueInterface,
	db *gorm.DB,
	paymentSvc *payment.PaymentService,
	walletSvc *wallet.WalletService,
) error {
	// Schedule recurring payment check
	recurringPaymentJob := NewRecurringPaymentJob(db, q, paymentSvc, walletSvc)
	if err := recurringPaymentJob.ScheduleRecurringPaymentCheck(); err != nil {
		return err
	}

	// Schedule auto-withdraw check
	autoWithdrawJob := NewAutoWithdrawJob(db, q)
	if _, err := autoWithdrawJob.ScheduleAutoWithdrawCheck(); err != nil {
		return err
	}

	// Schedule virtual account reconciliation
	virtualAccountJob := NewVirtualAccountJob(db, q, paymentSvc, walletSvc)
	if err := virtualAccountJob.ScheduleVirtualAccountReconciliation(); err != nil {
		return err
	}

	return nil
}
