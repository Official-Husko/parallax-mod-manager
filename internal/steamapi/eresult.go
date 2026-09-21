package steamapi

import "fmt"

// EResult is Steam's own status code, the number in the "result" field of every
// Workshop item answer (1 = success). The table below is the one Steamworks
// documents (partner.steamgames.com/doc/api/steam_api, "EResult"), kept whole so
// a log line or a status message can say what Steam meant by a code instead of
// printing a bare number.
type EResult int

const (
	ResultOK                 EResult = 1
	ResultFail               EResult = 2
	ResultNoConnection       EResult = 3
	ResultFileNotFound       EResult = 9
	ResultBusy               EResult = 10
	ResultAccessDenied       EResult = 15
	ResultTimeout            EResult = 16
	ResultBanned             EResult = 17
	ResultServiceUnavailable EResult = 20
	ResultInsufficientRights EResult = 24
	ResultLimitExceeded      EResult = 25
	ResultConnectFailed      EResult = 35
	ResultNoMatch            EResult = 42
	ResultRemoteCallFailed   EResult = 55
	ResultRateLimitExceeded  EResult = 84
	ResultItemDeleted        EResult = 86
)

type eresultInfo struct {
	name string
	text string
}

var eresults = map[EResult]eresultInfo{
	1:   {"OK", "Success."},
	2:   {"Fail", "Generic failure."},
	3:   {"NoConnection", "Your Steam client doesn't have a connection to the back-end."},
	5:   {"InvalidPassword", "Password/ticket is invalid."},
	6:   {"LoggedInElsewhere", "The user is logged in elsewhere."},
	7:   {"InvalidProtocolVer", "Protocol version is incorrect."},
	8:   {"InvalidParam", "A parameter is incorrect."},
	9:   {"FileNotFound", "File was not found."},
	10:  {"Busy", "Called method is busy - action not taken."},
	11:  {"InvalidState", "Called object was in an invalid state."},
	12:  {"InvalidName", "The name was invalid."},
	13:  {"InvalidEmail", "The email was invalid."},
	14:  {"DuplicateName", "The name is not unique."},
	15:  {"AccessDenied", "Access is denied."},
	16:  {"Timeout", "Operation timed out."},
	17:  {"Banned", "The user is VAC2 banned."},
	18:  {"AccountNotFound", "Account not found."},
	19:  {"InvalidSteamID", "The Steam ID was invalid."},
	20:  {"ServiceUnavailable", "The requested service is currently unavailable."},
	21:  {"NotLoggedOn", "The user is not logged on."},
	22:  {"Pending", "Request is pending, it may be in process or waiting on third party."},
	23:  {"EncryptionFailure", "Encryption or Decryption failed."},
	24:  {"InsufficientPrivilege", "Insufficient privilege."},
	25:  {"LimitExceeded", "Too much of a good thing."},
	26:  {"Revoked", "Access has been revoked (used for revoked guest passes.)"},
	27:  {"Expired", "License/Guest pass the user is trying to access is expired."},
	28:  {"AlreadyRedeemed", "Guest pass has already been redeemed by account, cannot be used again."},
	29:  {"DuplicateRequest", "The request is a duplicate and the action has already occurred in the past."},
	30:  {"AlreadyOwned", "All the games in this guest pass redemption request are already owned by the user."},
	31:  {"IPNotFound", "IP address not found."},
	32:  {"PersistFailed", "Failed to write change to the data store."},
	33:  {"LockingFailed", "Failed to acquire access lock for this operation."},
	34:  {"LogonSessionReplaced", "The logon session has been replaced."},
	35:  {"ConnectFailed", "Failed to connect."},
	36:  {"HandshakeFailed", "The authentication handshake has failed."},
	37:  {"IOFailure", "There has been a generic IO failure."},
	38:  {"RemoteDisconnect", "The remote server has disconnected."},
	39:  {"ShoppingCartNotFound", "Failed to find the shopping cart requested."},
	40:  {"Blocked", "A user blocked the action."},
	41:  {"Ignored", "The target is ignoring sender."},
	42:  {"NoMatch", "Nothing matching the request found."},
	43:  {"AccountDisabled", "The account is disabled."},
	44:  {"ServiceReadOnly", "This service is not accepting content changes right now."},
	45:  {"AccountNotFeatured", "Account doesn't have value, so this feature isn't available."},
	46:  {"AdministratorOK", "Allowed to take this action, but only because requester is admin."},
	47:  {"ContentVersion", "A Version mismatch in content transmitted within the Steam protocol."},
	48:  {"TryAnotherCM", "The current CM can't service the user making a request, user should try another."},
	49:  {"PasswordRequiredToKickSession", "You are already logged in elsewhere, this cached credential login has failed."},
	50:  {"AlreadyLoggedInElsewhere", "The user is logged in elsewhere."},
	51:  {"Suspended", "Long running operation has suspended/paused. (eg. content download.)"},
	52:  {"Cancelled", "Operation has been canceled, typically by user. (eg. a content download.)"},
	53:  {"DataCorruption", "Operation canceled because data is ill formed or unrecoverable."},
	54:  {"DiskFull", "Operation canceled - not enough disk space."},
	55:  {"RemoteCallFailed", "The remote or IPC call has failed."},
	56:  {"PasswordUnset", "Password could not be verified as it's unset server side."},
	57:  {"ExternalAccountUnlinked", "External account (PSN, Facebook...) is not linked to a Steam account."},
	58:  {"PSNTicketInvalid", "PSN ticket was invalid."},
	59:  {"ExternalAccountAlreadyLinked", "External account is already linked to some other account."},
	60:  {"RemoteFileConflict", "The sync cannot resume due to a conflict between the local and remote files."},
	61:  {"IllegalPassword", "The requested new password is not allowed."},
	62:  {"SameAsPreviousValue", "New value is the same as the old one."},
	63:  {"AccountLogonDenied", "Account login denied due to 2nd factor authentication failure."},
	64:  {"CannotUseOldPassword", "The requested new password is not legal."},
	65:  {"InvalidLoginAuthCode", "Account login denied due to auth code invalid."},
	66:  {"AccountLogonDeniedNoMail", "Account login denied due to 2nd factor auth failure - and no mail sent."},
	67:  {"HardwareNotCapableOfIPT", "The users hardware does not support Intel's Identity Protection Technology (IPT)."},
	68:  {"IPTInitError", "Intel's Identity Protection Technology (IPT) has failed to initialize."},
	69:  {"ParentalControlRestricted", "Operation failed due to parental control restrictions for current user."},
	70:  {"FacebookQueryError", "Facebook query returned an error."},
	71:  {"ExpiredLoginAuthCode", "Account login denied due to an expired auth code."},
	72:  {"IPLoginRestrictionFailed", "The login failed due to an IP restriction."},
	73:  {"AccountLockedDown", "The current users account is currently locked for use."},
	74:  {"AccountLogonDeniedVerifiedEmailRequired", "The logon failed because the accounts email is not verified."},
	75:  {"NoMatchingURL", "There is no URL matching the provided values."},
	76:  {"BadResponse", "Bad Response due to a Parse failure, missing field, etc."},
	77:  {"RequirePasswordReEntry", "The user cannot complete the action until they re-enter their password."},
	78:  {"ValueOutOfRange", "The value entered is outside the acceptable range."},
	79:  {"UnexpectedError", "Something happened that we didn't expect to ever happen."},
	80:  {"Disabled", "The requested service has been configured to be unavailable."},
	81:  {"InvalidCEGSubmission", "The files submitted to the CEG server are not valid."},
	82:  {"RestrictedDevice", "The device being used is not allowed to perform this action."},
	83:  {"RegionLocked", "The action could not be complete because it is region restricted."},
	84:  {"RateLimitExceeded", "Temporary rate limit exceeded, try again later."},
	85:  {"AccountLoginDeniedNeedTwoFactor", "Need two-factor code to login."},
	86:  {"ItemDeleted", "The thing we're trying to access has been deleted."},
	87:  {"AccountLoginDeniedThrottle", "Login attempt failed, try to throttle response to possible attacker."},
	88:  {"TwoFactorCodeMismatch", "Two factor authentication (Steam Guard) code is incorrect."},
	89:  {"TwoFactorActivationCodeMismatch", "The activation code for two-factor authentication didn't match."},
	90:  {"AccountAssociatedToMultiplePartners", "The current account has been associated with multiple partners."},
	91:  {"NotModified", "The data has not been modified."},
	92:  {"NoMobileDevice", "The account does not have a mobile device associated with it."},
	93:  {"TimeNotSynced", "The time presented is out of range or tolerance."},
	94:  {"SmsCodeFailed", "SMS code failure - no match, none pending, etc."},
	95:  {"AccountLimitExceeded", "Too many accounts access this resource."},
	96:  {"AccountActivityLimitExceeded", "Too many changes to this account."},
	97:  {"PhoneActivityLimitExceeded", "Too many changes to this phone."},
	98:  {"RefundToWallet", "Cannot refund to payment method, must use wallet."},
	99:  {"EmailSendFailure", "Cannot send an email."},
	100: {"NotSettled", "Can't perform operation until payment has settled."},
	101: {"NeedCaptcha", "The user needs to provide a valid captcha."},
	102: {"GSLTDenied", "A game server login token owned by this token's owner has been banned."},
	103: {"GSOwnerDenied", "Game server owner is denied for some other reason."},
	104: {"InvalidItemType", "The type of thing we were requested to act on is invalid."},
	105: {"IPBanned", "The IP address has been banned from taking this action."},
	106: {"GSLTExpired", "This Game Server Login Token has expired from disuse."},
	107: {"InsufficientFunds", "user doesn't have enough wallet funds to complete the action"},
	108: {"TooManyPending", "There are too many of this thing pending already"},
}

// Name is the Steamworks name of the code without its prefix ("FileNotFound"),
// or "Unknown" for a number the table does not have.
func (r EResult) Name() string {
	if info, ok := eresults[r]; ok {
		return info.name
	}
	return "Unknown"
}

// Description is Steamworks' one-line explanation of the code.
func (r EResult) Description() string {
	if info, ok := eresults[r]; ok {
		return info.text
	}
	return "Steam returned a result code that is not in its published list."
}

// String is the short form for logs and messages: "9 (FileNotFound)".
func (r EResult) String() string {
	return fmt.Sprintf("%d (%s)", int(r), r.Name())
}

// DescribeResult is String and Description together, for a message a person reads:
// "9 (FileNotFound): File was not found.".
func DescribeResult(code int) string {
	r := EResult(code)
	return r.String() + ": " + r.Description()
}

// IsOK says the item was returned in full.
func (r EResult) IsOK() bool { return r == ResultOK }

// IsNotFound says Steam has nothing to show for the item: FileNotFound and
// NoMatch. The free API answers this for some items whose Workshop page is up
// (an unlisted item, for one), so it is a reason to look elsewhere, never proof
// of a deletion.
func (r EResult) IsNotFound() bool { return r == ResultFileNotFound || r == ResultNoMatch }

// IsDeleted says Steam reports the item as deleted (ItemDeleted). Only the keyed
// API answers this; the free one folds it into FileNotFound.
func (r EResult) IsDeleted() bool { return r == ResultItemDeleted }

// IsDenied says the item exists but this caller may not see it: AccessDenied,
// InsufficientPrivilege and Banned.
func (r EResult) IsDenied() bool {
	return r == ResultAccessDenied || r == ResultInsufficientRights || r == ResultBanned
}

// IsRateLimit says Steam is throttling the caller: LimitExceeded and
// RateLimitExceeded.
func (r EResult) IsRateLimit() bool { return r == ResultLimitExceeded || r == ResultRateLimitExceeded }

// IsTransient says the answer is about Steam's side being unwell right now, not
// about the item: a failed or slow call, a busy or unavailable service. Nothing
// should be concluded about the item from it; asking again later is the answer.
func (r EResult) IsTransient() bool {
	switch r {
	case ResultFail, ResultNoConnection, ResultBusy, ResultTimeout, ResultServiceUnavailable,
		ResultConnectFailed, ResultRemoteCallFailed:
		return true
	}
	return false
}
