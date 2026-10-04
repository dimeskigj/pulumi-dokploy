package dokploy

import "github.com/pulumi/pulumi-go-provider/infer"

type NotificationEmailConfig struct {
	SMTPServer  string   `pulumi:"smtpServer"`
	SMTPPort    int      `pulumi:"smtpPort"`
	Username    string   `pulumi:"username"`
	Password    string   `pulumi:"password" provider:"secret"`
	FromAddress string   `pulumi:"fromAddress"`
	ToAddresses []string `pulumi:"toAddresses"`
}
type NotificationResendConfig struct {
	APIKey      string   `pulumi:"apiKey" provider:"secret"`
	FromAddress string   `pulumi:"fromAddress"`
	ToAddresses []string `pulumi:"toAddresses"`
}

func (a *NotificationEmailConfig) Annotate(n infer.Annotator) {
	n.Describe(&a.SMTPServer, "SMTP server hostname.")
	n.Describe(&a.SMTPPort, "SMTP port from 1 through 65535.")
	n.Describe(&a.Username, "SMTP username.")
	n.Describe(&a.Password, "Secret SMTP password.")
	n.Describe(&a.FromAddress, "Sender address.")
	n.Describe(&a.ToAddresses, "Nonempty list of recipient addresses.")
}
func (a *NotificationResendConfig) Annotate(n infer.Annotator) {
	n.Describe(&a.APIKey, "Secret Resend API key.")
	n.Describe(&a.FromAddress, "Sender address.")
	n.Describe(&a.ToAddresses, "Nonempty list of recipient addresses.")
}
