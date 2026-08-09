package commands

import "errors"

var (
	// ErrNotEnoughArguments is returned when a command is missing required arguments.
	ErrNotEnoughArguments = errors.New("not enough arguments")

	// ErrParsingArgument is returned when an argument that must be an integer can't be parsed.
	ErrParsingArgument = errors.New("error parsing arguments, please make sure all arguments are integers")

	// ErrNoPermission is returned when a user lacks permission to run a command.
	ErrNoPermission = errors.New("you don't have enough permission to execute this command")
)
