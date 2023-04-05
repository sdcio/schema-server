*** Settings ***
Library           OperatingSystem
Library           String
Library           Process
Suite Setup       Setup
Suite Teardown    Teardown

*** Variables ***
${server-bin}    ./schema-server
${client-bin}    ./client/client
${schema-server-config}    ./lab/distributed/schema-server.yaml
${data-server-config}    ./lab/distributed/data-server.yaml
${schema-server-ip}    127.0.0.1
${schema-server-port}    55000
${data-server-ip}    127.0.0.1
${data-server-port}    56000

# TARGET
${srlinux1-name}    srl1
${srlinux1-candidate}    default
${srlinux1-schema-name}    srl
${srlinux1-schema-version}    22.11.2
${srlinux1-schema-Vendor}    Nokia


# internal vars
${schema-server-process-alias}    ssa
${data-server-process-alias}    dsa
${data-server-stderr}    /tmp/ds-out



*** Test Cases ***
Check Server State
    CheckServerState

Set system0 admin-state disable
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=system0]/admin-state

    CreateCandidate    ${srlinux1-name}    ${srlinux1-candidate}
    ${result} =     Set    ${srlinux1-name}    ${srlinux1-candidate}    interface[name=system0]/admin-state:::disable

    Should Contain    ${result.stderr}    admin-state must be enable
    Should Be Equal As Integers    ${result.rc}    1

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Set lag-type without 'interface[name=xyz]/lag/lacp' existence
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=lag1]/lag/lag-type

    CreateCandidate    ${srlinux1-name}    ${srlinux1-candidate}
    ${result} =     Set    ${srlinux1-name}    ${srlinux1-candidate}    interface[name=lag1]/lag/lag-type:::lacp

    Should Contain    ${result.stderr}    lacp container must be configured when lag-type is lacp
    Should Be Equal As Integers    ${result.rc}    1

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Set lag-type with 'interface[name=xyz]/lag/lacp' existence
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=lag1]/lag/lacp/admin-key
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=lag1]/lag/lag-type

    CreateCandidate    ${srlinux1-name}    ${srlinux1-candidate}

    ${result} =     Set    ${srlinux1-name}    ${srlinux1-candidate}    interface[name=lag1]/lag/lacp/admin-key:::1
    Should Be Equal As Integers    ${result.rc}    0
    ${result} =     Set    ${srlinux1-name}    ${srlinux1-candidate}    interface[name=lag1]/lag/lag-type:::lacp
    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Set auto-negotiate on non allowed interface
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=ethernet-0/1]/ethernet/auto-negotiate

    CreateCandidate    ${srlinux1-name}    ${srlinux1-candidate}

    ${result} =     Set    ${srlinux1-name}    ${srlinux1-candidate}    interface[name=ethernet-0/1]/ethernet/auto-negotiate:::true
    Should Contain    ${result.stderr}    auto-negotiation not supported on this interface
    Should Be Equal As Integers    ${result.rc}    1

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Set auto-negotiate on allowed interface
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=ethernet-1/1]/ethernet/auto-negotiate

    CreateCandidate    ${srlinux1-name}    ${srlinux1-candidate}

    ${result} =     Set    ${srlinux1-name}    ${srlinux1-candidate}    interface[name=ethernet-1/1]/ethernet/auto-negotiate:::true
    Should Be Equal As Integers    ${result.rc}    0

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Set auto-negotiation on breakout-mode port
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=ethernet-1/1]/ethernet/auto-negotiate
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=ethernet-1/1]/breakout-mode/num-breakout-ports
    
    CreateCandidate    ${srlinux1-name}    ${srlinux1-candidate}
    
    ${result} =     Set    ${srlinux1-name}    ${srlinux1-candidate}    interface[name=ethernet-1/1]/breakout-mode/num-breakout-ports:::4
    Should Be Equal As Integers    ${result.rc}    0
    ${result} =     Set    ${srlinux1-name}    ${srlinux1-candidate}    interface[name=ethernet-1/1]/ethernet/auto-negotiate:::true
    Should Contain    ${result.stderr}    auto-negotiate not configurable when breakout-mode is enabled
    Should Be Equal As Integers    ${result.rc}    1

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}

Set breakout-port num to 2 and port-speed to 100G
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=ethernet-1/1]/breakout-mode/breakout-port-speed
    LogMustStatements    ${srlinux1-schema-name}    ${srlinux1-schema-version}    ${srlinux1-schema-vendor}    interface[name=ethernet-1/1]/breakout-mode/num-breakout-ports

    CreateCandidate    ${srlinux1-name}    ${srlinux1-candidate}
    
    ${result} =     Set    ${srlinux1-name}    ${srlinux1-candidate}    interface[name=ethernet-1/1]/breakout-mode/breakout-port-speed:::25G
    Should Be Equal As Integers    ${result.rc}    0

    ${result} =     Set    ${srlinux1-name}    ${srlinux1-candidate}    interface[name=ethernet-1/1]/breakout-mode/num-breakout-ports:::2
    Should Be Equal As Integers    ${result.rc}    1
        Should Contain    ${result.stderr}    breakout-port-speed must be 100G when num-breakout-ports is 2

    DeleteCandidate    ${srlinux1-name}    ${srlinux1-candidate}






*** Keywords ***
Setup
    [Documentation]    Starts schema and data server. Waits for the dataserver to begin sync before returning
    Start Process    ${server-bin}  -c     ${schema-server-config}    alias=${schema-server-process-alias}
    Start Process    ${server-bin}  -c     ${data-server-config}    alias=${data-server-process-alias}    stderr=${data-server-stderr}
    Wait Until Keyword Succeeds    180x    3s    WaitForOutput    ${data-server-stderr}    sync

Teardown
    [Documentation]    Stop all the started schema-server, data-server and client processes 
    Terminate All Processes

CheckServerState
    [Documentation]    Check that schema-server and data-server are still running
    Process Should Be Running    handle=${schema-server-process-alias}    error_message="schema-server failed"
    Process Should Be Running    handle=${data-server-process-alias}    error_message="data-server failed"

CreateCandidate
    [Arguments]    ${datastore}    ${candidate}
    ${result} =    Run Process    ${client-bin}     -a    ${data-server-ip}:${data-server-port}    datastore    create    --ds    ${datastore}    --candidate    ${candidate}
    RETURN    ${result}

DeleteCandidate
    [Arguments]    ${datastore}    ${candidate}
    ${result} =    Run Process    ${client-bin}     -a    ${data-server-ip}:${data-server-port}    datastore    delete    --ds    ${datastore}    --candidate    ${candidate}
    RETURN    ${result}

Commit
    [Documentation]    Performs a commit on the given datastore/candidate and returns the Process Result object https://robotframework.org/robotframework/latest/libraries/Process.html#Result%20object
    [Arguments]    ${datastore}    ${candidate}
    ${result} =    Run Process    ${client-bin}     -a    ${data-server-ip}:${data-server-port}    datastore    commit    --ds    ${datastore}    --candidate    ${candidate}
    RETURN    ${result}

Set
    [Documentation]    Applies to the candidate of the given datastore the provided update
    [Arguments]    ${datastore}    ${candidate}    ${update}
    ${result} =    Run Process    ${client-bin}     -a    ${data-server-ip}:${data-server-port}    data    set    --ds    ${datastore}    --candidate    ${candidate}    --update    ${update}
    Log    ${result.stdout}
    Log    ${result.stderr}
    RETURN    ${result}

GetSchema
    [Arguments]    ${name}    ${version}    ${vendor}    ${path}
    ${result} =    Run Process    ${client-bin}    -a    ${schema-server-ip}:${schema-server-port}    schema    get    --name    ${name}    --version    ${version}    --vendor    ${vendor}    --path    ${path}    
    RETURN    ${result}

ExtractResponse
    [Documentation]    Takes the output of the client binary and returns just the response part, stripping the request
    [Arguments]    ${output}
    @{split} =	Split String	${output}    response:
    RETURN    ${split}[1]

ExtractMustStatements
    [Arguments]    ${input}
    ${matches} =	Get Regexp Matches	${input}    must_statements:\\s*\{[\\s\\S]*?\}    flags=MULTILINE | IGNORECASE
    RETURN    ${matches}

LogMustStatements
    [Arguments]    ${name}    ${version}    ${vendor}    ${path}
    ${schema} =     GetSchema     ${name}    ${version}    ${vendor}    ${path}
    ${msts} =    ExtractMustStatements    ${schema.stdout}
    FOR    ${item}    IN    @{msts}
        Log    ${item}
    END

WaitForOutput
    [Arguments]    ${file}    ${pattern}
    ${ret} =	Grep File     ${file}    ${pattern}
    ${cnt}=    Get length    ${ret}
    IF    ${cnt} > 0
        RETURN
    ELSE
        Fail    Pattern (${pattern}) not found in file ${file}.    
    END    
    

