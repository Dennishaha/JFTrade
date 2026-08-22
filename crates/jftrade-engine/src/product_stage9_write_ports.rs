#[path = "product_execution_write_port.rs"]
mod product_execution_write_port;
use product_execution_write_port::{
    ExecutionWriteContext, ExecutionWritePort, ExecutionWriteRequest, ExecutionWriteResponse,
    dispatch_execution_write, execution_write_routes,
};
#[path = "product_system_write_port.rs"]
mod product_system_write_port;
use product_system_write_port::{
    SystemWritePort, SystemWriteRequest, SystemWriteResponse, dispatch_system_write,
    system_write_routes,
};
