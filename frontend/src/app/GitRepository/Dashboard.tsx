import * as React from 'react';
import { Form, FormGroup, TextInput, TextArea, Checkbox, Popover, ActionGroup, Button } from '@patternfly/react-core';
import HelpIcon from '@patternfly/react-icons/dist/js/icons/help-icon';
import { PageSection, Title } from '@patternfly/react-core';
import axios from 'axios';

const GitRepository: React.FunctionComponent = () => (
  <PageSection>
    <Title headingLevel="h1" size="lg">Create Git Repository</Title>

    <SimpleForm />

  </PageSection>
)


interface Props {}

interface State {
  name : string;
  repositoryURL : string;
  sshPrivateKey : string;
  value3 : string;
  // count: number;
};


export { GitRepository };

async function doThing() {

  const url: string = 'your-url.example';

  try {
    const response = await axios.get("http://localhost:9000");

    console.log(`${response.status}`)
  } catch (exception) {
    console.error(`ERROR received from ${url}: ${exception}\n`);
  }
}

class SimpleForm extends React.Component<Props, State> {

  constructor(props) {
    super(props);
    this.state = {
      name: '',
      repositoryURL: '',
      sshPrivateKey: '',
      value3: ''
    };

  }

  // handleInputChange(event : React.FormEvent<HTMLInputElement>) {
  //   const target = event.target;
  //   // const value = target.type === 'checkbox' ? target.checked : target.value;
  //   // const name : string = target.name;

  //   const { name, value} : any =  target
  //   // this.setState({ [name]: value    });
  // }


  handleNameFieldChange = name => {
    this.setState({ name });
  }

  handleRepoURLChange = repositoryURL => {
    this.setState({ repositoryURL });
  }

  

  handleTextInputChange1 = value3 => {
    this.setState({ value3 });
  }

  // handleTextInputChange3 = value3 => {
  //   this.setState({ value3 });
  // }

  render() {

    // doThing()

    const { name, repositoryURL, value3 } = this.state;

    return (
      <Form>
        <FormGroup
          label="Name"
          labelIcon={
            <Popover
              headerContent={
                <div>Name field
                </div>
              }
              bodyContent={
                <div>A unique, human-readable string representing the Git repository</div>
              }
            >
              <button
                type="button"
                aria-label="More info for name field"
                onClick={e => e.preventDefault()}
                aria-describedby="simple-form-name-01"
                className="pf-c-form__group-label-help"
              >
                <HelpIcon noVerticalAlign />
              </button>
            </Popover>
          }
          isRequired
          fieldId="simple-form-name-01"
          helperText="Please provide a name for the Git repository"
        >
          <TextInput
            isRequired
            type="text"
            id="simple-form-name-01"
            name="simple-form-name-01"
            aria-describedby="simple-form-name-01-helper"
            value={name}
          onChange={this.handleNameFieldChange}
          />
        </FormGroup>

        <FormGroup
          label="Repository URL"
          labelIcon={
            <Popover
              headerContent={
                <div>Repository URL</div>
              }
              bodyContent={
                <div>An SSH URL used to connect to the Git repository</div>
              }
            >
              <button
                type="button"
                aria-label="More info for repository field"
                onClick={e => e.preventDefault()}
                aria-describedby="simple-form-repository-02"
                className="pf-c-form__group-label-help"
              >
                <HelpIcon noVerticalAlign />
              </button>
            </Popover>
          }
          isRequired
          fieldId="simple-form-repository-02"
          helperText="Please provide a URL for the Git repository"
        >
          <TextInput
            isRequired
            type="text"
            id="simple-form-repository-02"
            name="simple-form-repository-02"
            aria-describedby="simple-form-repository-02-helper"
            value={repositoryURL}
          onChange={this.handleRepoURLChange}
          />
        </FormGroup>

        <FormGroup label="SSH Private Key" fieldId="simple-form-ssh-private-key-03">
          <TextArea
            value={value3}
            onChange={this.handleTextInputChange1}
            name="simple-form-ssh-private-key-03"
            id="simple-form-ssh-private-key-03"
          />
        </FormGroup>

        <FormGroup fieldId="simple-form-skip-server-verification-04" label="Skip server verification">
          <Checkbox 
            label="Skip verification of the server connection credentials" 
            aria-label="Skip verification of the server connection credentials" 
            id="simple-form-skip-server-verification-04" />
        </FormGroup>

        <FormGroup fieldId="simple-form-enable-lfs-support-05" label="Enable LFS support">
          <Checkbox 
            label="Enable Git LFS support" 
            aria-label="Enable Git LFS support" 
            id="simple-form-enable-lfs-support-05" />
        </FormGroup>

        <ActionGroup>
          <Button variant="primary">Submit form</Button>
          <Button variant="link">Cancel</Button>
        </ActionGroup>
      </Form>
    );
  }
}

