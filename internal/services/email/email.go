package email

import (
	"fmt"
	"log"
	"net/smtp"
	"os"
)

// EmailService handles sending emails
type EmailService struct {
	smtpHost     string
	smtpPort     string
	smtpUsername string
	smtpPassword string
	fromEmail    string
}

// NewEmailService creates a new email service
func NewEmailService() *EmailService {
	return &EmailService{
		smtpHost:     os.Getenv("SMTP_HOST"),
		smtpPort:     os.Getenv("SMTP_PORT"),
		smtpUsername: os.Getenv("SMTP_USERNAME"),
		smtpPassword: os.Getenv("SMTP_PASSWORD"),
		fromEmail:    os.Getenv("FROM_EMAIL"),
	}
}

// SendVerificationEmail sends an email with a verification link
func (s *EmailService) SendVerificationEmail(toEmail, username, token string) error {
	subject := "Verify Your RevasPay Account"
	verificationLink := fmt.Sprintf("%s/verify-email?token=%s", os.Getenv("FRONTEND_URL"), token)
	
	body := fmt.Sprintf(`
	<!DOCTYPE html>
	<html>
	<head>
		<style>
			body { font-family: Arial, sans-serif; line-height: 1.6; }
			.container { max-width: 600px; margin: 0 auto; padding: 20px; }
			.header { background-color: #4F46E5; color: white; padding: 10px; text-align: center; }
			.content { padding: 20px; }
			.button { display: inline-block; background-color: #4F46E5; color: white; padding: 10px 20px; text-decoration: none; border-radius: 5px; }
		</style>
	</head>
	<body>
		<div class="container">
			<div class="header">
				<h1>RevasPay</h1>
			</div>
			<div class="content">
				<h2>Hello %s,</h2>
				<p>Thank you for signing up with RevasPay! Please verify your email address to activate your account.</p>
				<p><a href="%s" class="button">Verify Email</a></p>
				<p>Or copy and paste this link in your browser: %s</p>
				<p>This link will expire in 48 hours.</p>
				<p>If you did not create an account with RevasPay, please ignore this email.</p>
				<p>Best regards,<br>The RevasPay Team</p>
			</div>
		</div>
	</body>
	</html>
	`, username, verificationLink, verificationLink)
	
	return s.sendEmail(toEmail, subject, body)
}

// SendPasswordResetEmail sends an email with a password reset link
func (s *EmailService) SendPasswordResetEmail(toEmail, username, token string) error {
	subject := "Reset Your RevasPay Password"
	resetLink := fmt.Sprintf("%s/reset-password?token=%s", os.Getenv("FRONTEND_URL"), token)
	
	body := fmt.Sprintf(`
	<!DOCTYPE html>
	<html>
	<head>
		<style>
			body { font-family: Arial, sans-serif; line-height: 1.6; }
			.container { max-width: 600px; margin: 0 auto; padding: 20px; }
			.header { background-color: #4F46E5; color: white; padding: 10px; text-align: center; }
			.content { padding: 20px; }
			.button { display: inline-block; background-color: #4F46E5; color: white; padding: 10px 20px; text-decoration: none; border-radius: 5px; }
		</style>
	</head>
	<body>
		<div class="container">
			<div class="header">
				<h1>RevasPay</h1>
			</div>
			<div class="content">
				<h2>Hello %s,</h2>
				<p>We received a request to reset your RevasPay password. Click the button below to create a new password:</p>
				<p><a href="%s" class="button">Reset Password</a></p>
				<p>Or copy and paste this link in your browser: %s</p>
				<p>This link will expire in 24 hours.</p>
				<p>If you did not request a password reset, please ignore this email or contact support if you have concerns.</p>
				<p>Best regards,<br>The RevasPay Team</p>
			</div>
		</div>
	</body>
	</html>
	`, username, resetLink, resetLink)
	
	return s.sendEmail(toEmail, subject, body)
}

// sendEmail sends an email with HTML content
func (s *EmailService) sendEmail(toEmail, subject, htmlBody string) error {
	if s.smtpHost == "" || s.smtpPort == "" || s.smtpUsername == "" || s.smtpPassword == "" {
		log.Println("Email service not configured properly. Check environment variables.")
		return fmt.Errorf("email service not configured")
	}

	mime := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"
	from := fmt.Sprintf("From: RevasPay <%s>\n", s.fromEmail)
	to := fmt.Sprintf("To: %s\n", toEmail)
	subject = fmt.Sprintf("Subject: %s\n", subject)
	
	message := []byte(from + to + subject + mime + htmlBody)
	
	auth := smtp.PlainAuth("", s.smtpUsername, s.smtpPassword, s.smtpHost)
	addr := fmt.Sprintf("%s:%s", s.smtpHost, s.smtpPort)
	
	return smtp.SendMail(addr, auth, s.fromEmail, []string{toEmail}, message)
}
