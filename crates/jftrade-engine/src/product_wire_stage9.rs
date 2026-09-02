impl ProductApi {
    fn is_stage9_write_path(&self, method: &str, path: &str) -> bool {
        is_market_data_subscription_mutation_path(method, path)
            || (is_brokers_write_path(method, path) && self.stage9_write_ports.brokers.is_some())
    }

    fn dispatch_stage9_write(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        if is_market_data_subscription_mutation_path(&request.method, &request.path) {
            return self.market_data_subscription_mutation.dispatch(request);
        }
        self.brokers_write(request)
    }
}
