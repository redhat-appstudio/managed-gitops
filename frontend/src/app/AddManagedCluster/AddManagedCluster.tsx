import * as React from 'react';
import { Form, FormGroup, TextInput, TextArea, Checkbox, Popover, ActionGroup, Button, SearchInput } from '@patternfly/react-core';
import HelpIcon from '@patternfly/react-icons/dist/js/icons/help-icon';
import { PageSection, Title } from '@patternfly/react-core';
import axios from 'axios';

// Name of cluster (a text input)
// Kube config (a multiline text area)
// Name of a context within the kube config (a text input)
// Submit button

const AddManagedCluster: React.FunctionComponent = () => (
  <PageSection>
    <Title headingLevel="h1" size="lg">Add Managed Cluster</Title>

    <SimpleForm />

  </PageSection>
)


interface Props {}

interface State {
  name : string;
  kubeconfig : string;
  context : string;
};


export { AddManagedCluster };

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
      kubeconfig: '',
      context: '',
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

  handleRepoURLChange = kubeconfig => {
    this.setState({ kubeconfig });
  }

  

  handleTextInputChange1 = context => {
    this.setState({ context });
  }

  render() {

    // doThing()

    const { name, kubeconfig, context } = this.state;

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
                <div>An unqiue, human-readable string representing your Cluster Name</div>
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
          helperText="Please provide a name for creating a Cluster"
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
          label="KubeConfig Detail"
          labelIcon={
            <Popover
              headerContent={
                <div>KubeConfig Detail</div>
              }
              bodyContent={
                <div>Please paste your kubeconfig details here, required for creating a cluster</div>
              }
            >
              <button
                type="button"
                aria-label="More info for kubeconfig detail field"
                onClick={e => e.preventDefault()}
                aria-describedby="simple-form-kubeconfig-02"
                className="pf-c-form__group-label-help"
              >
                <HelpIcon noVerticalAlign />
              </button>
            </Popover>
          }
          isRequired
          fieldId="simple-form-kubeconfig-02"
          helperText="Please provide a kubeConfig for creating a cluster"
        >
          <TextArea
            isRequired
            type="text"
            id="simple-form-kubeconfig-02"
            name="simple-form-kubeconfig-02"
            aria-describedby="simple-form-kubeconfig-02-helper"
            value={kubeconfig}
          onChange={this.handleRepoURLChange}
          />
        </FormGroup>

        <FormGroup 
        label="Context Name" 
        labelIcon={
          <Popover
            headerContent={
              <div>Context Name</div>
            }
            bodyContent={
              <div>Please give name of a context present within the kube config</div>
            }
          >
            <button
              type="button"
              aria-label="More info for context field"
              onClick={e => e.preventDefault()}
              aria-describedby="simple-form-context-02"
              className="pf-c-form__group-label-help"
            >
              <HelpIcon noVerticalAlign />
            </button>
          </Popover>
        }
        isRequired
        fieldId="simple-form-context-03"
        helperText="Please provide a context name for creating a cluster"
      >
          <TextInput
            value={context}
            onChange={this.handleTextInputChange1}
            name="simple-form-context-03"
            id="simple-form-context-03"
          />
        </FormGroup>

        <ActionGroup>
          <Button variant="primary">Submit form</Button>
          <Button variant="link">Cancel</Button>
        </ActionGroup>
      </Form>
    );
  }
}

// class SearchInputWithResultCount extends React.Component {
//   constructor(props) {
//     super(props);
//     this.state = {
//       value: '',
//       resultsCount: 0
//     };

//     this.onChange = (value, event) => {
//       this.setState({
//         value: value,
//         resultsCount: 3
//       });
//     };

//     this.onClear = (event) => {
//       this.setState({
//         value: '',
//         resultsCount: 0
//       });
//     }
//   }

//   render() {
//     return (
//       <SearchInput
//         placeholder='Find by name'
//         value={this.state.value}
//         onChange={this.onChange}
//         onClear={this.onClear}
//         resultsCount={this.state.resultsCount}
//       />
//     );
//   }
// }