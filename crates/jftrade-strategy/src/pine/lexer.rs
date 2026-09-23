//! A small, lossless lexer for the executable Pine v6 subset.
//!
//! Pine uses indentation for blocks.  The lexer therefore keeps one token
//! stream per physical line instead of inserting synthetic braces.  This is
//! deliberately a lexer (strings, comments, operators and escapes are
//! recognised here); the parser is responsible for indentation and grammar.

use thiserror::Error;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum TokenKind {
    Identifier,
    Number,
    String,
    Operator,
    Punctuation,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Token {
    pub kind: TokenKind,
    pub lexeme: String,
    pub line: usize,
    pub column: usize,
    pub end_column: usize,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct LexedLine {
    pub number: usize,
    /// Indentation width in columns (a tab counts as four columns), used for
    /// block structure.
    pub indent: usize,
    /// Number of leading whitespace characters. Token columns are absolute
    /// positions in the raw line, so slicing the trimmed `text` needs this
    /// character offset (`indent` differs from it when tabs are used).
    pub offset: usize,
    pub text: String,
    pub tokens: Vec<Token>,
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum LexError {
    #[error("pine line {line}, column {column}: unterminated string literal")]
    UnterminatedString { line: usize, column: usize },
    #[error("pine line {line}, column {column}: invalid character {character:?}")]
    InvalidCharacter {
        line: usize,
        column: usize,
        character: char,
    },
    #[error("pine line {line}, column {column}: invalid numeric literal")]
    InvalidNumber { line: usize, column: usize },
}

pub fn lex(source: &str) -> Result<Vec<LexedLine>, LexError> {
    source
        .lines()
        .enumerate()
        .map(|(index, raw)| lex_line(index + 1, raw))
        .collect()
}

fn lex_line(number: usize, raw: &str) -> Result<LexedLine, LexError> {
    let mut indent = 0;
    let mut offset = 0;
    for character in raw.chars() {
        match character {
            ' ' => {
                indent += 1;
                offset += 1;
            }
            '\t' => {
                indent += 4;
                offset += 1;
            }
            _ => break,
        }
    }
    let text = raw.trim().to_owned();
    if text.is_empty() || text.starts_with("//") {
        return Ok(LexedLine {
            number,
            indent,
            offset,
            text,
            tokens: Vec::new(),
        });
    }

    let characters = raw.chars().collect::<Vec<_>>();
    let mut cursor = offset;
    let mut tokens = Vec::new();
    while cursor < characters.len() {
        let character = characters[cursor];
        if character.is_whitespace() {
            cursor += 1;
            continue;
        }
        if character == '/' && characters.get(cursor + 1) == Some(&'/') {
            break;
        }
        let column = cursor + 1;
        if character == '"' || character == '\'' {
            // Go's tokenizer treats single and double quotes as string
            // delimiters (``stripInlineComment`` switches on both), so a
            // single-quoted literal such as ``'close comment'`` is data, not
            // an invalid character.
            let delimiter = character;
            let start = cursor;
            cursor += 1;
            let mut escaped = false;
            let mut terminated = false;
            while cursor < characters.len() {
                let current = characters[cursor];
                cursor += 1;
                if escaped {
                    escaped = false;
                } else if current == '\\' {
                    escaped = true;
                } else if current == delimiter {
                    terminated = true;
                    break;
                }
            }
            if !terminated {
                return Err(LexError::UnterminatedString {
                    line: number,
                    column,
                });
            }
            let lexeme = characters[start..cursor].iter().collect::<String>();
            tokens.push(Token {
                kind: TokenKind::String,
                lexeme,
                line: number,
                column,
                end_column: cursor + 1,
            });
            continue;
        }
        if character.is_ascii_digit()
            || (character == '.'
                && characters
                    .get(cursor + 1)
                    .is_some_and(|next| next.is_ascii_digit()))
        {
            let start = cursor;
            let mut dots = 0;
            while cursor < characters.len() {
                let current = characters[cursor];
                if current == '.' {
                    dots += 1;
                    if dots > 1 {
                        return Err(LexError::InvalidNumber {
                            line: number,
                            column,
                        });
                    }
                } else if !current.is_ascii_digit() {
                    break;
                }
                cursor += 1;
            }
            let lexeme = characters[start..cursor].iter().collect::<String>();
            tokens.push(Token {
                kind: TokenKind::Number,
                lexeme,
                line: number,
                column,
                end_column: cursor + 1,
            });
            continue;
        }
        if character.is_ascii_alphabetic() || character == '_' {
            let start = cursor;
            cursor += 1;
            while cursor < characters.len()
                && (characters[cursor].is_ascii_alphanumeric() || characters[cursor] == '_')
            {
                cursor += 1;
            }
            let lexeme = characters[start..cursor].iter().collect::<String>();
            tokens.push(Token {
                kind: TokenKind::Identifier,
                lexeme,
                line: number,
                column,
                end_column: cursor + 1,
            });
            continue;
        }

        let two = characters
            .get(cursor..cursor.saturating_add(2))
            .map(|slice| slice.iter().collect::<String>());
        if let Some(operator) = two.filter(|value| {
            matches!(
                value.as_str(),
                "=>" | ":=" | "==" | "!=" | "<=" | ">=" | "&&" | "||"
            )
        }) {
            cursor += 2;
            tokens.push(Token {
                kind: TokenKind::Operator,
                lexeme: operator,
                line: number,
                column,
                end_column: cursor + 1,
            });
            continue;
        }
        if matches!(
            character,
            '+' | '-' | '*' | '/' | '%' | '>' | '<' | '=' | '!'
        ) {
            cursor += 1;
            tokens.push(Token {
                kind: TokenKind::Operator,
                lexeme: character.to_string(),
                line: number,
                column,
                end_column: cursor + 1,
            });
            continue;
        }
        if matches!(
            character,
            '.' | ',' | '(' | ')' | '[' | ']' | '{' | '}' | ':' | '?'
        ) {
            cursor += 1;
            tokens.push(Token {
                kind: TokenKind::Punctuation,
                lexeme: character.to_string(),
                line: number,
                column,
                end_column: cursor + 1,
            });
            continue;
        }
        return Err(LexError::InvalidCharacter {
            line: number,
            column,
            character,
        });
    }
    Ok(LexedLine {
        number,
        indent,
        offset,
        text,
        tokens,
    })
}

pub fn decode_string(token: &str) -> String {
    // Go's ``unquote`` tries ``strconv.Unquote`` first and then falls back to
    // stripping a matching pair of delimiters.  For single-quoted literals the
    // fallback is what a strategy author observes (`'close comment'` becomes
    // `close comment`), so Rust strips the delimiters without escape handling
    // for that form.
    if let Some(inner) = token
        .strip_prefix('\'')
        .and_then(|value| value.strip_suffix('\''))
    {
        return inner.to_owned();
    }
    let inner = token
        .strip_prefix('"')
        .and_then(|value| value.strip_suffix('"'))
        .unwrap_or(token);
    let mut output = String::with_capacity(inner.len());
    let mut escaped = false;
    for character in inner.chars() {
        if escaped {
            output.push(match character {
                'n' => '\n',
                'r' => '\r',
                't' => '\t',
                '\\' => '\\',
                '"' => '"',
                other => other,
            });
            escaped = false;
        } else if character == '\\' {
            escaped = true;
        } else {
            output.push(character);
        }
    }
    if escaped {
        output.push('\\');
    }
    output
}
