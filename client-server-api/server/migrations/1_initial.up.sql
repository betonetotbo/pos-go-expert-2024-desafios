create table exchanges (
    id         integer primary key autoincrement,
    Code       varchar(255),
	Codein     varchar(255),
	Name       varchar(255),
	High       float64,
	Low        float64,
    VarBid     float64,
    PctChange  float64,
    Bid        float64,
    Ask        float64,
    "Timestamp"  timestamp,
    CreateDate date
)
