/// Process-local cancellation fan-out for active provider calls.
#[derive(Debug, Default)]
pub(crate) struct RunCancellationRegistry {
    active: Mutex<BTreeMap<String, Vec<Arc<AtomicBool>>>>,
    stopping: AtomicBool,
}

impl RunCancellationRegistry {
    fn register(&self, run_id: &str) -> Arc<AtomicBool> {
        let token = Arc::new(AtomicBool::new(false));
        self.register_token(run_id, token)
    }

    fn register_token(&self, run_id: &str, token: Arc<AtomicBool>) -> Arc<AtomicBool> {
        if let Ok(mut active) = self.active.lock() {
            if self.stopping.load(Ordering::Acquire) {
                token.store(true, Ordering::Release);
            }
            active
                .entry(run_id.to_owned())
                .or_default()
                .push(Arc::clone(&token));
        }
        token
    }

    fn unregister(&self, run_id: &str, token: &Arc<AtomicBool>) {
        if let Ok(mut active) = self.active.lock() {
            let remove_run = active.get_mut(run_id).is_some_and(|tokens| {
                tokens.retain(|candidate| !Arc::ptr_eq(candidate, token));
                tokens.is_empty()
            });
            if remove_run {
                active.remove(run_id);
            }
        }
    }

    pub(crate) fn cancel(&self, run_id: &str) -> bool {
        let tokens = self
            .active
            .lock()
            .ok()
            .and_then(|active| active.get(run_id).cloned())
            .unwrap_or_default();
        for token in &tokens {
            token.store(true, Ordering::Release);
        }
        !tokens.is_empty()
    }

    #[allow(dead_code)]
    fn cancel_all(&self) {
        if let Ok(active) = self.active.lock() {
            // Serialize late registrations with shutdown cancellation. A
            // provider admitted earlier must not miss this fan-out.
            self.stopping.store(true, Ordering::Release);
            for tokens in active.values() {
                for token in tokens {
                    token.store(true, Ordering::Release);
                }
            }
        }
    }
}
