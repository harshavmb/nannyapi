# Contributing to NannyAPI

Thank you for your interest in contributing to NannyAPI! This document provides guidelines and instructions for contributing.

## Development Process

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to your branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Code Style Guidelines

- Follow standard Go coding conventions
- Use `gofmt` to format your code
- Run `go vet` and `staticcheck` before committing
- Maintain test coverage above 80%
- Document all exported functions and types
- Keep functions small and focused
- Use meaningful variable names

## Testing

- Write tests for all new features
- Maintain or improve code coverage
- Run the full test suite before submitting a PR
- Include both unit and integration tests where appropriate

## Documentation

- Update API documentation when modifying endpoints
- Run `swag init` to update Swagger docs
- Keep README.md up to date
- Document complex algorithms and business logic
- Include examples in documentation where helpful

## Pull Request Process

1. Follow the PR template
2. Ensure all tests pass
3. Update relevant documentation
4. Add test cases for new functionality
5. Request review from maintainers
6. Address review comments promptly

## Commit Messages

- Use clear and meaningful commit messages
- Follow conventional commits format:
  - feat: New feature
  - fix: Bug fix
  - docs: Documentation changes
  - test: Adding tests
  - refactor: Code refactoring
  - style: Formatting changes
  - chore: Maintenance tasks

## Code Review

- Be respectful and constructive in reviews
- Focus on code quality and correctness
- Consider performance implications
- Check for security issues
- Verify documentation updates

## Getting Help

- Create an issue for bug reports
- Use discussions for questions
- Tag maintainers for urgent issues
- Join our community chat for real-time help

## Security Issues

- Report security vulnerabilities privately
- Do not create public issues for security bugs
- Contact maintainers directly
- Follow responsible disclosure practices

## License

By contributing, you agree that your contributions will be licensed under the GNU General Public License v3.0.
