# Mermaid diagram examples

## Flowchart

```mermaid
graph TD
    A[Start] --> B{Safe to deploy?}
    B -->|yes| C[Ship it]
    B -->|no| D[Roll back]
    C --> E((Done))
    D --> A
```

```mermaid
flowchart TD
    client[Client] --> |request| LB[Load balancer]
    LB --> api1[/API one/]
    LB --> api2[API two]
    api1 --> db[(Primary DB)]
    api2 -.->|failover| db
    api2 -. retry .-> LB
```

## Sequence diagram

```mermaid
sequenceDiagram
    participant A as Alice
    participant B as Bob
    A->B: Hello Bob
    B-->>A: Hi Alice, long time!
    A->A: What now?
    Note right of A: thinking hard
    alt is morning
        A->>B: Good morning!
    else is evening
        B->A: Good evening...
    end
    Note over A,B: done chatting
```

## Same-rank and self loop

```mermaid
flowchart TD
    A[Ingest] --> B[Queue]
    A --> C[Wal]
    B --- C
    B -->|tick| B
    B --> D[[Archive]]
    D --> E{{compact}}
```

## Shapes

```mermaid
flowchart TD
    q([quick]) --> r[rect]
    r --> s(round)
    s --> t[[double]]
    t --> u((circle))
    u --> v{rhombus}
    v --> w{{hexagon}}
    w --> x[(cylinder)]
```

## LR direction and multi-line labels

```mermaid
flowchart LR
    S["collector sources"] --> C["ingest<br/>validate, deduplicate"]
    C --> D[("durable store<br/>replayable")]
    D --> W["writer commits"]
    D -.->|"204 relay progress"| C
```

## Subgraph frames

```mermaid
flowchart TD
    subgraph svc["svc"]
        subgraph hot["hot"]
            A["ingest"]
            B["enrich"]
        end
        C[("spool")]
    end
    X["outside"]
    A --> B --> C
    C --> X
```

## Unsupported (kept as source)

```mermaid
graph BT
    A --> B --> C --> D
```
