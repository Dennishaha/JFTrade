//! Bounded workflow invocation ownership. Polling cancellation alone does not stop jobs.
use std::collections::BTreeMap;
use std::sync::{
    Mutex,
    atomic::{AtomicBool, Ordering},
};
use std::thread::JoinHandle;
use std::time::{Duration, Instant};

#[derive(Debug, Default)]
pub(crate) struct WorkflowJobs {
    stopping: AtomicBool,
    tasks: Mutex<BTreeMap<String, JoinHandle<()>>>,
}

impl WorkflowJobs {
    pub(crate) fn spawn(&self, id: String, job: impl FnOnce() + Send + 'static) -> bool {
        let mut tasks = self.tasks.lock().unwrap_or_else(|e| e.into_inner());
        let finished: Vec<_> = tasks
            .iter()
            .filter(|(_, h)| h.is_finished())
            .map(|(id, _)| id.clone())
            .collect();
        for id in finished {
            if let Some(h) = tasks.remove(&id) {
                let _ = h.join();
            }
        }
        if self.stopping.load(Ordering::Acquire) || tasks.contains_key(&id) || tasks.len() >= 8 {
            return false;
        }
        match std::thread::Builder::new()
            .name("workflow-invocation".to_owned())
            .spawn(job)
        {
            Ok(handle) => {
                tasks.insert(id, handle);
                true
            }
            Err(error) => {
                tracing::error!(%error, "could not start workflow invocation");
                false
            }
        }
    }

    pub(crate) fn stop(&self) {
        let _tasks = self.tasks.lock().unwrap_or_else(|e| e.into_inner());
        self.stopping.store(true, Ordering::Release);
    }

    pub(crate) fn is_stopping(&self) -> bool {
        self.stopping.load(Ordering::Acquire)
    }

    pub(crate) fn join(&self, timeout: Duration) -> bool {
        self.stop();
        let deadline = Instant::now() + timeout;
        let mut tasks = self.tasks.lock().unwrap_or_else(|e| e.into_inner());
        while tasks.values().any(|h| !h.is_finished()) && Instant::now() < deadline {
            std::thread::sleep(Duration::from_millis(10));
        }
        let finished: Vec<_> = tasks
            .iter()
            .filter(|(_, h)| h.is_finished())
            .map(|(id, _)| id.clone())
            .collect();
        for id in finished {
            if let Some(h) = tasks.remove(&id) {
                let _ = h.join();
            }
        }
        tasks.is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn stop_blocks_new_dispatch_and_join_reports_active_owners() {
        let jobs = WorkflowJobs::default();
        let (tx, rx) = std::sync::mpsc::channel();
        assert!(jobs.spawn("one".into(), move || {
            let _ = rx.recv();
        }));
        jobs.stop();
        assert!(!jobs.spawn("two".into(), || panic!("must not run")));
        assert!(!jobs.join(Duration::from_millis(20)));
        tx.send(()).unwrap();
        assert!(jobs.join(Duration::from_secs(1)));
    }
}
