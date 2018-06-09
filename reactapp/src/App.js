import React, {Component} from 'react';
import logo from './logo.svg';
import './App.css';
import Eos from 'eosjs';

class App extends Component {
    state = {};

    constructor(props) {
        super(props);
        this.eos = Eos();
        this.eos.getInfo({}).then(res => {
            this.setState({info: res})
        });
        setInterval(() => {
            this.eos.getInfo({}).then(res => {
                this.setState({info: res})
            });
        }, 1000);
    }

    componentDidMount() {
        this.window = window;
    }

    getIdentity = () => {
        this.window.scatter.getIdentity().then(identity => {
            console.info('Identity: ' + identity)
            alert('Identity: ' + identity.name);
        }).catch(error => {
            console.error(error)
            alert('Error: ' + error.message);
        });
    };

    render() {
        const info = this.state.info;
        return info ? (
            <div className="App">
                <div className="App-intro" style={{padding: 10}}>
                    <h3>EOS</h3>
                    <table>
                        <tbody>
                        {Object.keys(info).map(k =>
                            <tr align="left" key={k}>
                                <td>{k}</td>
                                <td>{info[k]}</td>
                            </tr>
                        )}
                        </tbody>
                    </table>
                    <button type={'button'} onClick={this.getIdentity}>Get Identity</button>
                </div>
            </div>
        ) : null;
    }
}

export default App;
