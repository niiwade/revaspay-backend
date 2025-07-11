package momo

// PaymentNotification represents a payment notification from MTN MoMo webhook
type PaymentNotification struct {
	ReferenceID  string `json:"referenceId"`
	Status       string `json:"status"`
	ExternalID   string `json:"externalId"`
	Amount       string `json:"amount"`
	Currency     string `json:"currency"`
	PayerMessage string `json:"payerMessage"`
	PayeeNote    string `json:"payeeNote"`
	Timestamp    string `json:"timestamp"`
}

// DisbursementNotification represents a disbursement notification from MTN MoMo webhook
type DisbursementNotification struct {
	ReferenceID  string `json:"referenceId"`
	Status       string `json:"status"`
	ExternalID   string `json:"externalId"`
	Amount       string `json:"amount"`
	Currency     string `json:"currency"`
	PayerMessage string `json:"payerMessage"`
	PayeeNote    string `json:"payeeNote"`
	Timestamp    string `json:"timestamp"`
}
