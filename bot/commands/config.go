package commands

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/VTGare/Eugen/bot"
	"github.com/VTGare/Eugen/store"
	"github.com/VTGare/Eugen/utils"
	"github.com/VTGare/embeds"
	"github.com/bwmarrin/discordgo"
)

func ban(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageCreate, []string) error {
	return func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
		ok, err := utils.MemberHasPermission(s, m.GuildID, m.Author.ID, discordgo.PermissionAdministrator|discordgo.PermissionManageServer)
		if err != nil {
			return err
		}

		if !ok {
			return fmt.Errorf("You don't have enough permissions to run this command.")
		}

		if len(args) == 0 {
			return utils.ErrNotEnoughArguments
		}

		guild := b.Store.Guilds.Cache().Get(m.GuildID)

		banned := make([]string, 0)
		for _, arg := range args {
			if strings.HasPrefix(arg, "<#") {
				arg = strings.Trim(arg, "#<>")
			}

			ch, err := s.Channel(arg)
			if err != nil {
				if strings.HasPrefix(err.Error(), "403") {
					return fmt.Errorf("Unable to get channel: <#%v>. Not enough permissions.", arg)
				}
				return err
			}

			if ch.GuildID == m.GuildID {
				if !guild.IsBanned(ch.ID) {
					err := b.Store.Guilds.BanChannel(context.Background(), ch.GuildID, ch.ID)
					if err != nil {
						return err
					}

					banned = append(banned, fmt.Sprintf("<#%v>", ch.ID))
				}
			}
		}

		embed := utils.BaseEmbed(s)
		embed.Title = "✅ Successfully banned channels"
		embed.Description = fmt.Sprintf("List of banned channels:\n%v", banned)
		s.ChannelMessageSendEmbed(m.ChannelID, embed)
		return nil
	}
}

func unban(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageCreate, []string) error {
	return func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
		ok, err := utils.MemberHasPermission(s, m.GuildID, m.Author.ID, discordgo.PermissionAdministrator|discordgo.PermissionManageServer)
		if err != nil {
			return err
		}

		if !ok {
			return fmt.Errorf("You don't have enough permissions to run this command.")
		}

		if len(args) == 0 {
			return utils.ErrNotEnoughArguments
		}

		guild := b.Store.Guilds.Cache().Get(m.GuildID)
		unbanned := make([]string, 0)
		for _, arg := range args {
			arg = strings.Trim(arg, "<#>")

			if slices.Contains(guild.BannedChannels, arg) {
				err = b.Store.Guilds.UnbanChannel(context.Background(), guild.ID, arg)
				if err != nil {
					return err
				}

				unbanned = append(unbanned, fmt.Sprintf("<#%v>", arg))
			}
		}

		embed := utils.BaseEmbed(s)
		if len(unbanned) > 0 {
			embed.Title = "✅ Successfully unbanned channels"
			embed.Description = fmt.Sprintf("List of unbanned channels:\n%v", unbanned)
		} else {
			embed.Title = "❎ Failed to unban channels"
			embed.Description = "No channels were unbanned"
		}
		s.ChannelMessageSendEmbed(m.ChannelID, embed)
		return nil
	}
}

func blacklist(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageCreate, []string) error {
	return func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
		ok, err := utils.MemberHasPermission(s, m.GuildID, m.Author.ID, discordgo.PermissionAdministrator|discordgo.PermissionManageServer)
		if err != nil {
			return err
		}

		if !ok {
			return fmt.Errorf("You don't have enough permissions to run this command.")
		}

		if len(args) == 0 {
			return utils.ErrNotEnoughArguments
		}

		guild := b.Store.Guilds.Cache().Get(m.GuildID)
		blacklisted := make([]string, 0)
		for _, arg := range args {
			arg = strings.Trim(arg, "<@!>")

			user, err := s.User(arg)
			if err != nil {
				b.Logger().Warn("fetching user", "err", err)
			}

			err = b.Store.Guilds.BanUser(context.Background(), guild.ID, user.ID)
			if err != nil {
				b.Logger().Warn("banning user", "err", err)
			}

			blacklisted = append(blacklisted, fmt.Sprintf("<@%v>", arg))
		}

		embed := utils.BaseEmbed(s)
		embed.Title = "✅ Successfully blacklisted users"
		embed.Description = fmt.Sprintf("List of blacklisted users:\n%v", blacklisted)
		s.ChannelMessageSendEmbed(m.ChannelID, embed)
		return nil
	}
}

func unblacklist(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageCreate, []string) error {
	return func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
		ok, err := utils.MemberHasPermission(s, m.GuildID, m.Author.ID, discordgo.PermissionAdministrator|discordgo.PermissionManageServer)
		if err != nil {
			return err
		}

		if !ok {
			return fmt.Errorf("You don't have enough permissions to run this command.")
		}

		if len(args) == 0 {
			return utils.ErrNotEnoughArguments
		}

		guild := b.Store.Guilds.Cache().Get(m.GuildID)
		unblacklisted := make([]string, 0)
		for _, arg := range args {
			arg = strings.Trim(arg, "<@!>")

			if slices.Contains(guild.BlacklistedUsers, arg) {
				err = b.Store.Guilds.UnbanUser(context.Background(), guild.ID, arg)
				if err != nil {
					return err
				}

				unblacklisted = append(unblacklisted, fmt.Sprintf("<@%v>", arg))
			}
		}

		embed := utils.BaseEmbed(s)
		embed.Title = "✅ Successfully unblacklisted users"
		embed.Description = fmt.Sprintf("List of unblacklisted users:\n%v", unblacklisted)
		s.ChannelMessageSendEmbed(m.ChannelID, embed)
		return nil
	}
}

func req(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageCreate, []string) error {
	return func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
		ok, err := utils.MemberHasPermission(s, m.GuildID, m.Author.ID, discordgo.PermissionAdministrator|discordgo.PermissionManageServer)
		if err != nil {
			return err
		}

		if !ok {
			return fmt.Errorf("You don't have enough permissions to run this command.")
		}

		if len(args) < 2 {
			return utils.ErrNotEnoughArguments
		}

		g := b.Store.Guilds.Cache().Get(m.GuildID)

		channelID := strings.Trim(args[0], "<#>")

		if !slices.ContainsFunc(g.ChannelSettings, func(ch *store.ChannelSettings) bool {
			return ch.ID == channelID
		}) {
			if !utils.IsValidChannel(s, m.GuildID, channelID) {
				return fmt.Errorf("Unable to get channel <#%v>. Please make sure Eugen has permissions to see the channel.", channelID)
			}
		}

		if args[1] == "default" {
			if slices.ContainsFunc(g.ChannelSettings, func(ch *store.ChannelSettings) bool {
				return ch.ID == channelID
			}) {
				err = b.Store.Guilds.UnsetChannelStars(context.Background(), m.GuildID, channelID)
				if err != nil {
					return err
				}
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Successfully reset <#%v> settings to defaults", channelID))
			} else {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Can't reset <#%v> to defaults, channel doesn't have star requirements set.", channelID))
			}
		} else {
			stars, err := strconv.Atoi(args[1])
			if err != nil {
				return utils.ErrParsingArgument
			}
			if stars < 1 {
				return fmt.Errorf("Star requirement should be >= 1, provided star requirement is %v", stars)
			}

			err = b.Store.Guilds.SetChannelStars(context.Background(), m.GuildID, channelID, stars)
			if err != nil {
				return fmt.Errorf("store error\n%v", err)
			}
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Successfully set <#%v> star requirement to %v", channelID, stars))
		}
		return nil
	}
}

func set(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageCreate, []string) error {
	return func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
		switch len(args) {
		case 0:
			showGuildSettings(s, m, b)
		case 2:
			isAdmin, err := utils.MemberHasPermission(s, m.GuildID, m.Author.ID, discordgo.PermissionAdministrator)
			if err != nil {
				return err
			}

			if !isAdmin {
				return utils.ErrNoPermission
			}

			setting := args[0]
			newSetting := strings.ToLower(args[1])

			var passedSetting any
			switch setting {
			case "enabled":
				passedSetting, err = strconv.ParseBool(newSetting)
			case "selfstar":
				passedSetting, err = strconv.ParseBool(newSetting)
			case "ignorebots":
				passedSetting, err = strconv.ParseBool(newSetting)
			case "color":
				if passedSetting, err = strconv.ParseInt(newSetting, 0, 32); err != nil {
					if passedSetting, err = strconv.ParseInt("0x"+newSetting, 0, 32); err != nil {
						return fmt.Errorf("unable to parse %v to a number", newSetting)
					}
				}
				if passedSetting.(int64) > 16777215 || passedSetting.(int64) < 0 {
					return errors.New("non-existing decimal color, it should be in range from 0 to 16777215")
				}
			case "prefix":
				if unicode.IsLetter(rune(newSetting[len(newSetting)-1])) {
					passedSetting = newSetting + " "
				} else {
					passedSetting = newSetting
				}
				if len(passedSetting.(string)) > 5 {
					return errors.New("new prefix is too long")
				}
			case "stars":
				passedSetting, err = strconv.Atoi(newSetting)
			case "emote":
				emoji, err := utils.GetEmoji(s, m.GuildID, newSetting)
				if err != nil {
					return errors.New("argument's either global emoji or not one at all")
				}
				passedSetting = emoji
			case "starboard":
				if chID, ok := strings.CutPrefix(newSetting, "<#"); ok {
					newSetting = strings.TrimSuffix(chID, ">")
				}
				ch, err := s.Channel(newSetting)
				if err != nil {
					return err
				}
				if ch.GuildID != m.GuildID {
					return errors.New("can't assign starboard to a channel from a foreign server")
				}
				passedSetting = newSetting
			default:
				return errors.New("unknown setting " + setting)
			}

			if err != nil {
				return err
			}

			_, err = b.Store.Guilds.SetField(context.Background(), m.GuildID, setting, passedSetting)
			if err != nil {
				return err
			}

			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Successfully changed ``%v`` to ``%v``", setting, newSetting))
		default:
			return errors.New("incorrect command usage. Please use e!help set command for more information")
		}

		return nil
	}
}

func showGuildSettings(s *discordgo.Session, m *discordgo.MessageCreate, b *bot.Bot) {
	settings := b.Store.Guilds.Cache().Get(m.GuildID)
	guild, _ := s.Guild(settings.ID)

	banned := strings.Join(utils.Map(settings.BannedChannels, func(s string) string {
		return fmt.Sprintf("<#%v>", s)
	}), " | ")
	if banned == "" {
		banned = "none"
	}

	eb := embeds.NewBuilder().
		Title("Current settings").
		Description(guild.Name).
		Color(int(settings.EmbedColor)).
		Thumbnail(guild.IconURL("320")).
		Timestamp(time.Now())

	eb.AddField("Starboard", fmt.Sprintf("**%v**\n**Starboard channel:** %v", utils.FormatBool(settings.Enabled), utils.FormatChannel(settings.StarboardChannel)), false)
	eb.AddField("General settings", fmt.Sprintf("**Emote:** %v | **Prefix:** %v | **Color:** %v", settings.StarEmote, settings.Prefix, settings.EmbedColor), false)
	eb.AddField("Behavior settings", fmt.Sprintf("**Selfstar:** %v | **Ignore bots:** %v | **Min stars:** %v", utils.FormatBool(settings.Selfstar), utils.FormatBool(settings.IgnoreBots), settings.MinimumStars), false)
	eb.AddField("Unique star requirements", settings.ChannelSettingsToString(), false)
	eb.AddField("Blacklisted users", settings.BlacklistedToString(), false)
	eb.AddField("Banned channels", settings.BannedChannelsToString(), false)

	s.ChannelMessageSendEmbed(m.ChannelID, eb.Finalize())
}

// verifyStarboardChannel validates that the given string is a channel mention
// for a channel that exists in the same guild as the invoking message.
// It returns the resolved channel ID (without the <#...> wrapper).
func verifyStarboardChannel(s *discordgo.Session, guildID, chID string) (string, bool) {
	if !strings.HasPrefix(chID, "<#") || !strings.HasSuffix(chID, ">") {
		return "", false
	}

	cut, _ := strings.CutPrefix(chID, "<#")
	chID = strings.TrimSuffix(cut, ">")

	ch, err := s.Channel(chID)
	if err != nil {
		return "", false
	}

	if ch.GuildID != guildID {
		return "", false
	}

	return chID, true
}

// parseColor validates and parses a colour value from user input, accepting
// both decimal and hexadecimal (with or without 0x prefix) representations.
// The value must be in the range [0, 16777215] to be a valid Discord colour.
func parseColor(colour string) (int64, bool) {
	c, err := strconv.ParseInt(colour, 0, 32)
	if err != nil {
		c, err = strconv.ParseInt("0x"+colour, 0, 32)
		if err != nil {
			return 0, false
		}
	}

	if c > 16777215 || c < 0 {
		return 0, false
	}

	return c, true
}

// setupStep is a closure executed during the interactive setup wizard. It
// returns (advanced, error): advanced==true moves to the next step, false
// aborts the setup.
type setupStep func() (advanced bool, err error)

func setup(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageCreate, []string) error {
	return func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
		ok, err := utils.MemberHasPermission(s, m.GuildID, m.Author.ID, discordgo.PermissionAdministrator|discordgo.PermissionManageGuild)
		if err != nil {
			return err
		}

		if !ok {
			return errors.New("You don't have permission to run this command.")
		}

		var (
			guild     = b.Store.Guilds.Cache().Get(m.GuildID)
			step      = 0
			done      = false
			exit      = false
			starboard = ""
			selfstar  = false
			minstars  = 0
			emote     = ""
			color     = int64(0)
		)

		steps := []setupStep{
			// Step 1: Starboard channel
			func() (bool, error) {
				eb := embeds.NewBuilder().
					Title("Eugen Setup | Step 1: Starboard channel").
					Description("To complete this step **mention a starboard channel.**\n\nType ``cancel`` or ``exit`` to quit the setup.").
					Thumbnail(s.State.User.AvatarURL("")).
					Color(utils.EmbedColor).
					Timestamp(time.Now())

				res := ""
				for !(res == "cancel" || res == "exit") {
					res = utils.CreatePrompt(s, m, eb.Finalize())

					if chID, ok := verifyStarboardChannel(s, m.GuildID, res); ok {
						starboard = "<#" + chID + ">"
						break
					}
				}

				if res == "cancel" || res == "exit" {
					return false, nil
				}

				step++
				return true, nil
			},

			// Step 2: Minimum stars
			func() (bool, error) {
				eb := embeds.NewBuilder().
					Title("Eugen Setup | Step 2: Minimum stars").
					Description(fmt.Sprintf("**Current settings:**\nStarboard channel: %v\n\nTo complete this step **type an integer number**.\n\nType ``cancel`` or ``exit`` to cancel the setup.\nType ``previous`` to come back to a previous step", starboard)).
					Thumbnail(s.State.User.AvatarURL("")).
					Color(utils.EmbedColor).
					Timestamp(time.Now())

				res := ""
				for !(res == "cancel" || res == "exit" || res == "previous") {
					res = utils.CreatePrompt(s, m, eb.Finalize())

					if num, err := strconv.Atoi(res); err == nil {
						minstars = num
						break
					}
				}

				if res == "cancel" || res == "exit" {
					return false, nil
				}

				if res == "previous" {
					step--
					return true, nil
				}

				step++
				return true, nil
			},

			// Step 3: Star emote
			func() (bool, error) {
				eb := embeds.NewBuilder().
					Title("Eugen Setup | Step 3: Star emote").
					Description(fmt.Sprintf("**Current settings:**\nStarboard channel: %v\nMinimum stars: %v\n\nTo complete this step **send a guild emote or type default.**\n\nType ``cancel`` or ``exit`` to cancel the setup.\nType ``previous`` to come back to a previous step", starboard, minstars)).
					Thumbnail(s.State.User.AvatarURL("")).
					Color(utils.EmbedColor).
					Timestamp(time.Now())

				res := ""
				for !(res == "cancel" || res == "exit" || res == "previous" || res == "default") {
					res = utils.CreatePrompt(s, m, eb.Finalize())

					if e, err := utils.GetEmoji(s, m.GuildID, res); err == nil {
						emote = e
						break
					}
				}

				if res == "cancel" || res == "exit" {
					return false, nil
				}

				if res == "default" {
					emote = "⭐"
				}

				if res == "previous" {
					step--
					return true, nil
				}

				step++
				return true, nil
			},

			// Step 4: Selfstar
			func() (bool, error) {
				eb := embeds.NewBuilder().
					Title("Eugen Setup | Step 4: Selfstar").
					Description(fmt.Sprintf("**Current settings:**\nStarboard channel: %v\nMinimum stars: %v\nEmote: %v\n\nTo complete this step **type ``true`` to allow self-starring or ``false`` to not count self-stars.**\n\nType ``cancel`` or ``exit`` to cancel the setup.\nType ``previous`` to come back to a previous step", starboard, minstars, emote)).
					Thumbnail(s.State.User.AvatarURL("")).
					Color(utils.EmbedColor).
					Timestamp(time.Now())

				res := ""
				for !(res == "true" || res == "false" || res == "cancel" || res == "exit" || res == "previous") {
					res = utils.CreatePrompt(s, m, eb.Finalize())
				}

				if res == "cancel" || res == "exit" {
					return false, nil
				}

				switch res {
				case "true":
					selfstar = true
				case "false":
					selfstar = false
				}

				if res == "previous" {
					step--
					return true, nil
				}

				step++
				return true, nil
			},

			// Step 5: Embed color
			func() (bool, error) {
				eb := embeds.NewBuilder().
					Title("Eugen Setup | Step 5: Embed color").
					Description(fmt.Sprintf("**Current settings:**\nStarboard channel: %v\nMinimum stars: %v\nEmote: %v\nSelf-star: %v\n\nTo complete this step **send a hexadecimal color or integer up to 16777215 or default.**\n\nType ``cancel`` or ``exit`` to cancel the setup.\nType ``previous`` to come back to a previous step", starboard, minstars, emote, utils.FormatBool(selfstar))).
					Thumbnail(s.State.User.AvatarURL("")).
					Color(utils.EmbedColor).
					Timestamp(time.Now())

				res := ""
				for !(res == "cancel" || res == "exit" || res == "previous" || res == "default") {
					res = utils.CreatePrompt(s, m, eb.Finalize())

					if c, ok := parseColor(res); ok {
						color = c
						break
					}
				}

				if res == "cancel" || res == "exit" {
					return false, nil
				}

				if res == "default" {
					color = 16744576
				}

				if res == "previous" {
					step--
					return true, nil
				}

				done = true
				return true, nil
			},
		}

		for !done {
			advanced, err := steps[step]()
			if err != nil {
				return err
			}

			if !advanced {
				exit = true
				done = true
			}
		}

		eb := embeds.NewBuilder().
			Color(utils.EmbedColor).
			Timestamp(time.Now())

		if !exit {
			guild.Enabled = true
			guild.StarboardChannel = strings.TrimSuffix(strings.TrimPrefix(starboard, "<#"), ">")
			guild.MinimumStars = minstars
			guild.StarEmote = emote
			guild.Selfstar = selfstar
			guild.EmbedColor = color
			guild.UpdatedAt = time.Now()

			err = b.Store.Guilds.Replace(context.Background(), guild)
			if err != nil {
				eb = eb.FailureTemplate(fmt.Sprintf("Error occurred while saving settings.\n\n%v", err))
			} else {
				eb = eb.SuccessTemplate("Successfully setup Eugen!").
					Color(int(color)).
					AddField("Starboard channel", starboard, true).
					AddField("Minimum stars", fmt.Sprintf("%v", minstars), true).
					AddField("Emote", emote, true).
					AddField("Self-star", utils.FormatBool(selfstar), true).
					AddField("Embed color", "applied to this embed :)", true)
			}
		} else {
			eb = eb.FailureTemplate("Failed to setup Eugen.")
		}

		s.ChannelMessageSendEmbed(m.ChannelID, eb.Finalize())
		return nil
	}
}
