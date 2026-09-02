use std::collections::{BTreeMap, BTreeSet};

use thiserror::Error;

use crate::{WorkflowTask, WorkflowTaskStatus};

#[derive(Debug, Error, Eq, PartialEq)]
pub enum WorkflowError {
    #[error("workflow task id must not be empty")]
    EmptyTaskId,
    #[error("workflow task {0} is duplicated")]
    DuplicateTask(String),
    #[error("workflow task {task} depends on missing task {dependency}")]
    MissingDependency { task: String, dependency: String },
    #[error("workflow task {0} depends on itself")]
    SelfDependency(String),
    #[error("workflow task graph contains a cycle")]
    Cycle,
    #[error("workflow task {0} is not ready")]
    NotReady(String),
    #[error("workflow task {0} cannot be completed from its current status")]
    CannotComplete(String),
}

#[derive(Clone, Debug, Default)]
pub struct TaskGraph {
    tasks: BTreeMap<String, WorkflowTask>,
}

impl TaskGraph {
    pub fn new(tasks: Vec<WorkflowTask>) -> Result<Self, WorkflowError> {
        let mut indexed = BTreeMap::new();
        for task in tasks {
            let id = task.id.trim().to_owned();
            if id.is_empty() {
                return Err(WorkflowError::EmptyTaskId);
            }
            if indexed.insert(id.clone(), task).is_some() {
                return Err(WorkflowError::DuplicateTask(id));
            }
        }
        for (id, task) in &indexed {
            for dependency in &task.depends_on {
                if dependency == id {
                    return Err(WorkflowError::SelfDependency(id.clone()));
                }
                if !indexed.contains_key(dependency) {
                    return Err(WorkflowError::MissingDependency {
                        task: id.clone(),
                        dependency: dependency.clone(),
                    });
                }
            }
        }
        if has_cycle(&indexed) {
            return Err(WorkflowError::Cycle);
        }
        Ok(Self { tasks: indexed })
    }

    pub fn tasks(&self) -> Vec<WorkflowTask> {
        let mut tasks = self.tasks.values().cloned().collect::<Vec<_>>();
        tasks.sort_by(|left, right| (left.order, &left.id).cmp(&(right.order, &right.id)));
        tasks
    }

    pub fn ready_task(&self) -> Option<WorkflowTask> {
        self.tasks().into_iter().find(|task| self.is_ready(task))
    }

    pub fn claim(&mut self, task_id: &str) -> Result<WorkflowTask, WorkflowError> {
        let ready = self
            .tasks
            .get(task_id)
            .is_some_and(|task| self.is_ready(task));
        if !ready {
            return Err(WorkflowError::NotReady(task_id.to_owned()));
        }
        let task = self.tasks.get_mut(task_id).expect("ready task exists");
        task.status = WorkflowTaskStatus::InProgress;
        Ok(task.clone())
    }

    pub fn complete(
        &mut self,
        task_id: &str,
        summary: impl Into<String>,
    ) -> Result<WorkflowTask, WorkflowError> {
        let task = self
            .tasks
            .get_mut(task_id)
            .ok_or_else(|| WorkflowError::CannotComplete(task_id.to_owned()))?;
        if !matches!(
            task.status,
            WorkflowTaskStatus::Todo | WorkflowTaskStatus::InProgress
        ) {
            return Err(WorkflowError::CannotComplete(task_id.to_owned()));
        }
        task.status = WorkflowTaskStatus::Done;
        task.result_summary = summary.into().trim().to_owned();
        Ok(task.clone())
    }

    pub fn block(
        &mut self,
        task_id: &str,
        reason: impl Into<String>,
    ) -> Result<WorkflowTask, WorkflowError> {
        let task = self
            .tasks
            .get_mut(task_id)
            .ok_or_else(|| WorkflowError::CannotComplete(task_id.to_owned()))?;
        if matches!(
            task.status,
            WorkflowTaskStatus::Done | WorkflowTaskStatus::Cancelled
        ) {
            return Err(WorkflowError::CannotComplete(task_id.to_owned()));
        }
        task.status = WorkflowTaskStatus::Blocked;
        task.result_summary = reason.into().trim().to_owned();
        Ok(task.clone())
    }

    fn is_ready(&self, task: &WorkflowTask) -> bool {
        task.status == WorkflowTaskStatus::Todo
            && task.depends_on.iter().all(|dependency| {
                self.tasks
                    .get(dependency)
                    .is_some_and(|task| task.status == WorkflowTaskStatus::Done)
            })
    }
}

fn has_cycle(tasks: &BTreeMap<String, WorkflowTask>) -> bool {
    fn visit(
        id: &str,
        tasks: &BTreeMap<String, WorkflowTask>,
        visiting: &mut BTreeSet<String>,
        visited: &mut BTreeSet<String>,
    ) -> bool {
        if visited.contains(id) {
            return false;
        }
        if !visiting.insert(id.to_owned()) {
            return true;
        }
        let cyclic = tasks[id]
            .depends_on
            .iter()
            .any(|dependency| visit(dependency, tasks, visiting, visited));
        visiting.remove(id);
        visited.insert(id.to_owned());
        cyclic
    }

    let mut visiting = BTreeSet::new();
    let mut visited = BTreeSet::new();
    tasks
        .keys()
        .any(|id| visit(id, tasks, &mut visiting, &mut visited))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn graph_exposes_one_deterministic_ready_task() {
        let mut graph = TaskGraph::new(vec![
            WorkflowTask {
                id: "second".to_owned(),
                title: "Second".to_owned(),
                status: WorkflowTaskStatus::Todo,
                depends_on: vec!["first".to_owned()],
                order: 2,
                result_summary: String::new(),
            },
            WorkflowTask {
                id: "first".to_owned(),
                title: "First".to_owned(),
                status: WorkflowTaskStatus::Todo,
                depends_on: Vec::new(),
                order: 1,
                result_summary: String::new(),
            },
        ])
        .expect("graph");
        assert_eq!(graph.ready_task().expect("ready").id, "first");
        graph.complete("first", "done").expect("complete");
        assert_eq!(graph.ready_task().expect("ready").id, "second");
    }
}
