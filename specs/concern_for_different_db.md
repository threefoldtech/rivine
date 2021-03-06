# Overview

Right now the internal database used is [bbolt, a fork of boltdb](https://github.com/coreos/bbolt). At some point, we might have a reason to change this DB.
Multiple modules currently have a DB, the consensus set, explorer, transaction pool, possibly others in the future. Each DB is completely isolated, they have no
knowledge of eachother, and communication happens entirey via the application. It should be noted that these databases are single application, i.e. only one process
at a time can access them. Any other process looking ot simultaniously access the info will need to use the deamon.

## Concerns when changing to a different database

Although each module can in theory have a different database, some things should be noted before attempting to change the implementation:

- the current code base does not fully abstract the database

    Bolt uses transactions, which guarantee atomicity. Part of the code leverages this feature, and has the database logic woven into the application logic. Although this is
    definitely not required everywhere, there might be occasions where this feature is actively leveraged (think corrupted consensus set in case of power loss, for instance).
    This raises 2 concerns:

        * The application and database logic must become separated first before a new DB can be introduced
        * In instances where said atomicity is a major attribution, a system must be deviced where the DB is abstracted while still exposing features to provide said atomicity

- Using a non embedded DB increases setup complexity

    Right now the only thing required to run is the deamon itself. If we were to use a non embedded db, users first need to run said DB, and possibly do some configuring. Also,
    this would likely require the user to add extra configuration to the deamon to inform it how to reach said db.
