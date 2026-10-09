use std::cmp::Ordering;

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct SkillToolValidation {
    pub status: &'static str,
    pub message: String,
}

pub fn validate_skill_tools(
    builtin: bool,
    requested: &[String],
    available: &[String],
) -> SkillToolValidation {
    let unknown = requested
        .iter()
        .filter(|tool| !available.contains(tool))
        .cloned()
        .collect::<Vec<_>>();
    if builtin || unknown.is_empty() {
        SkillToolValidation {
            status: "VALID",
            message: String::new(),
        }
    } else {
        SkillToolValidation {
            status: "WARNING",
            message: format!("unknown ADK tools: {}", unknown.join(", ")),
        }
    }
}

pub fn compare_skill_catalog_order(
    left_source: &str,
    left_display_name: &str,
    right_source: &str,
    right_display_name: &str,
) -> Ordering {
    left_source
        .cmp(right_source)
        .then_with(|| left_display_name.cmp(right_display_name))
}
