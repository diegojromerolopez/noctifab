# Specification: Quick Math Calculator CLI

## Project Overview
A fast, lightweight CLI calculator utility written in Go.

## Functional Requirements
- `Add(a, b int) int`: Returns the sum of two integers.
- `Subtract(a, b int) int`: Returns the difference of two integers.
- `Multiply(a, b int) int`: Returns the product of two integers.
- `Divide(a, b int) (int, error)`: Returns integer division result, returning an error when dividing by zero.

## Quality & Testing Criteria
- 100% test coverage for all arithmetic operations and edge cases (divide by zero).
- Code must pass standard Go formatting and linting.
