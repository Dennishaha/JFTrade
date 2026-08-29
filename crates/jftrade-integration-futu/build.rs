fn main() {
    let protos = [
        "../../pkg/futu/proto/Common.proto",
        "../../pkg/futu/proto/Qot_Common.proto",
        "../../pkg/futu/proto/Qot_GetSecuritySnapshot.proto",
        "../../pkg/futu/proto/Qot_RequestHistoryKL.proto",
        "../../pkg/futu/proto/Trd_Common.proto",
        "../../pkg/futu/proto/Trd_GetAccList.proto",
        "../../pkg/futu/proto/Trd_GetFunds.proto",
        "../../pkg/futu/proto/Trd_GetPositionList.proto",
        "../../pkg/futu/proto/Trd_GetOrderList.proto",
        "../../pkg/futu/proto/Trd_GetOrderFillList.proto",
        "../../pkg/futu/proto/Trd_GetOrderFee.proto",
        "../../pkg/futu/proto/Trd_GetMarginRatio.proto",
        "../../pkg/futu/proto/Trd_GetMaxTrdQtys.proto",
        "../../pkg/futu/proto/Trd_FlowSummary.proto",
    ];
    for proto in protos {
        println!("cargo:rerun-if-changed={proto}");
    }
    tonic_prost_build::configure()
        .build_client(false)
        .build_server(false)
        .compile_protos(&protos, &["../../pkg/futu/proto"])
        .expect("compile Futu trade protos");
}
